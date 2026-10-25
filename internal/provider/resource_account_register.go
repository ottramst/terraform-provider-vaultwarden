package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AccountRegister{}
var _ resource.ResourceWithConfigure = &AccountRegister{}
var _ resource.ResourceWithImportState = &AccountRegister{}

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
	// PasswordWO is write-only: it is only set in the configuration, never in the plan or state
	PasswordWO        types.String `tfsdk:"password_wo"`
	PasswordWOVersion types.Int64  `tfsdk:"password_wo_version"`
}

// accountReplaceWarning is appended to the descriptions of attributes that force replacement.
const accountReplaceWarning = " Changing it deletes the account, including its vault, and registers a new one."

func (r *AccountRegister) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_register"
}

func (r *AccountRegister) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource registers a new account on the Vaultwarden server.\n\n" +
			"`password` is saved in plain text to the state. With Terraform 1.11 and later, use the write-only `password_wo` " +
			"with `password_wo_version` instead, which are never stored.\n\nRequires `admin_token` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the registered account",
				PlanModifiers: []planmodifier.String{
					// Without it, an in-place update would plan the ID as unknown, and an unknown
					// ID used to force a replacement
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the account to register." + accountReplaceWarning,
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The email of the account to register." + accountReplaceWarning,
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "The password of the account to register. It is stored in plain text in the state: " +
					"with Terraform 1.11 and later, use `password_wo` instead. Exactly one of `password` and `password_wo` must be set." +
					accountReplaceWarning +
					" The password cannot be read back, so it is not set after an import; setting it then only updates the state." +
					" Moving the same password to `password_wo` does not replace the account.",
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("password_wo")),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						accountPasswordRequiresReplace,
						"Changing the password of a registered account requires replacing it.",
						"Changing the password of a registered account requires replacing it.",
					),
				},
			},
			"password_wo": schema.StringAttribute{
				MarkdownDescription: "The password of the account to register, as a write-only attribute that is never stored " +
					"in the plan or state. Requires Terraform 1.11 or later. Exactly one of `password` and `password_wo` must be set. " +
					"Terraform cannot detect changes of a write-only value: change `password_wo_version` to register the account " +
					"again with a new password.",
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
			},
			"password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version of `password_wo`. Changing it deletes the account, including its vault, and " +
					"registers a new one with the current `password_wo`. Setting it for the first time, for example after an " +
					"import or when moving from `password` to `password_wo`, only updates the state.",
				Optional: true,
				Validators: []validator.Int64{
					int64validator.AlsoRequires(path.MatchRoot("password_wo")),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplaceIf(
						func(ctx context.Context, req planmodifier.Int64Request, resp *int64planmodifier.RequiresReplaceIfFuncResponse) {
							// Only a changed version replaces the account. There is no version in state after an
							// import or a move from password, and none in the plan after a move back to password.
							resp.RequiresReplace = !req.StateValue.IsNull() && !req.PlanValue.IsNull() && !req.PlanValue.Equal(req.StateValue)
						},
						"Changing the password version of a registered account replaces it with the current password_wo.",
						"Changing the password version of a registered account replaces it with the current `password_wo`.",
					),
				},
			},
		},
	}
}

// accountPasswordRequiresReplace replaces a registered account when its password changes. Only an
// imported account has no password in state, and moving the password unchanged to password_wo
// keeps the account.
func accountPasswordRequiresReplace(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	if req.StateValue.IsNull() {
		return
	}

	if req.PlanValue.IsNull() {
		var passwordWO types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("password_wo"), &passwordWO)...)
		if passwordWO.Equal(req.StateValue) {
			return
		}
	}

	resp.RequiresReplace = true
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

	// The write-only password is only in the configuration
	password := data.Password.ValueString()
	if data.Password.IsNull() {
		var passwordWO types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("password_wo"), &passwordWO)...)
		if resp.Diagnostics.HasError() {
			return
		}
		password = passwordWO.ValueString()
	}

	// New accounts use the default KDF settings
	kdfConfig := models.DefaultKdfConfiguration()

	// Build prelogin key
	preloginKey, err := keybuilder.BuildPreloginKey(password, data.Email.ValueString(), kdfConfig)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error building prelogin key",
			"Could not build prelogin key, unexpected error: "+err.Error(),
		)
		return
	}

	// Hash password
	hashedPw := crypt.HashPassword(password, *preloginKey, false)

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
	data.Name = types.StringValue(userResp.Name)

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
		// Deleted outside Terraform
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
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

func (r *AccountRegister) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
