package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AccountRegister{}

func AccountRegisterResource() resource.Resource {
	return &AccountRegister{}
}

// AccountRegister defines the resource implementation.
type AccountRegister struct {
	client *vaultwarden.Client
}

// AccountRegisterModel describes the resource data model.
type AccountRegisterModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Email    types.String `tfsdk:"email"`
	Password types.String `tfsdk:"password"`
}

func (r *AccountRegister) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_register"
}

func (r *AccountRegister) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource registers a new account on the Vaultwarden server.\n\nThis resource will save the password in plain text to the state! Use caution!",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the registered account",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the account to register",
				Optional:            true,
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The email of the account to register",
				Required:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "The password of the account to register",
				Required:            true,
				Sensitive:           true,
			},
		},
	}
}

func (r *AccountRegister) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*vaultwarden.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vaultwarden.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *AccountRegister) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AccountRegisterModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Do prelogin to get KDF parameters
	preloginResp, err := r.client.PreLogin(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error prelogin",
			"Could not prelogin, unexpected error: "+err.Error(),
		)
		return
	}

	// Build the KDF configuration
	kdfConfig := &models.KdfConfiguration{
		KdfType:        preloginResp.Kdf,
		KdfIterations:  preloginResp.KdfIterations,
		KdfMemory:      preloginResp.KdfMemory,
		KdfParallelism: preloginResp.KdfParallelism,
	}

	// Build prelogin key
	preloginKey, err := keybuilder.BuildPreloginKey(data.Password.ValueString(), data.Email.ValueString(), kdfConfig)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error building prelogin key",
			"Could not build prelogin key, unexpected error: "+err.Error(),
		)
		return
	}

	// Hash password
	hashedPw := crypt.HashPassword(data.Password.ValueString(), *preloginKey, false)

	// Try to log in first - if successful, the user already exists
	if _, err := r.client.LoginWithUserCredentials(ctx, hashedPw); err == nil {
		resp.Diagnostics.AddError(
			"User already exists",
			"Cound not create user, user already exists",
		)
		return
	}

	// Create encryption key
	encryptionKey, encryptedEncryptionKey, err := keybuilder.GenerateEncryptionKey(*preloginKey)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error generating encryption key",
			"Could not generate encryption key, unexpected error: "+err.Error(),
		)
		return
	}

	// Generate public/private key pair
	publicKey, encryptedPrivateKey, err := keybuilder.GenerateEncryptedRSAKeyPair(*encryptionKey)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error generating RSA key pair",
			"Could not generate RSA key pair, unexpected error: "+err.Error(),
		)
		return
	}

	// Call the client method to invite the user
	registerReq := vaultwarden.RegisterUserRequest{
		Name:               data.Name.ValueString(),
		Email:              data.Email.ValueString(),
		MasterPasswordHash: hashedPw,
		Key:                encryptedEncryptionKey,
		Kdf:                kdfConfig.KdfType,
		KdfIterations:      kdfConfig.KdfIterations,
		KdfMemory:          kdfConfig.KdfMemory,
		KdfParallelism:     kdfConfig.KdfParallelism,
		Keys: models.KeyPair{
			PublicKey:           publicKey,
			EncryptedPrivateKey: encryptedPrivateKey,
		},
	}

	if err := r.client.RegisterUser(ctx, registerReq); err != nil {
		resp.Diagnostics.AddError(
			"Error registering user",
			"Could not register user, unexpected error: "+err.Error(),
		)
		return
	}

	// Fetch the registered account by email
	userResp, err := r.client.GetUserByEmail(ctx, data.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error fetching registered user",
			"Could not fetch registered user, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response body to schema and populate Computed attribute values
	data.ID = types.StringValue(userResp.ID)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, fmt.Sprintf("created a new user_invite with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AccountRegister) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AccountRegisterModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed data from the client
	userResp, err := r.client.GetUser(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden user",
			"Could not read user with ID "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Overwrite the model with the refreshed data
	data.Name = types.StringValue(userResp.Name)
	data.Email = types.StringValue(userResp.Email)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AccountRegister) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AccountRegisterModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AccountRegister) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AccountRegisterModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Delete the user
	if err := r.client.DeleteUser(ctx, data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden user",
			"Could not delete user with ID "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}
}
