package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &OrganizationEventsDataSource{}
var _ datasource.DataSourceWithConfigure = &OrganizationEventsDataSource{}

func NewOrganizationEventsDataSource() datasource.DataSource {
	return &OrganizationEventsDataSource{}
}

// OrganizationEventsDataSource reads the event log of an organization.
type OrganizationEventsDataSource struct {
	client *vaultwarden.Client
}

// OrganizationEventsDataSourceModel describes the vaultwarden_organization_events data source.
type OrganizationEventsDataSourceModel struct {
	OrganizationID types.String              `tfsdk:"organization_id"`
	Start          types.String              `tfsdk:"start"`
	End            types.String              `tfsdk:"end"`
	Events         []OrganizationEventsModel `tfsdk:"events"`
}

// OrganizationEventsModel describes an event of an organization.
type OrganizationEventsModel struct {
	Type               types.Int64  `tfsdk:"type"`
	TypeName           types.String `tfsdk:"type_name"`
	Date               types.String `tfsdk:"date"`
	ActingUserID       types.String `tfsdk:"acting_user_id"`
	UserID             types.String `tfsdk:"user_id"`
	OrganizationUserID types.String `tfsdk:"organization_user_id"`
	ItemID             types.String `tfsdk:"item_id"`
	CollectionID       types.String `tfsdk:"collection_id"`
	GroupID            types.String `tfsdk:"group_id"`
	PolicyID           types.String `tfsdk:"policy_id"`
	DeviceType         types.Int64  `tfsdk:"device_type"`
	IPAddress          types.String `tfsdk:"ip_address"`
}

func (d *OrganizationEventsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_events"
}

func (d *OrganizationEventsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to read the event log of an organization between two dates, such as logins, " +
			"changes to items, collections, groups and members, and policy updates. The provider user needs the `Owner` or `Admin` role in the organization.\n\n" +
			"Vaultwarden only logs events when `ORG_EVENTS_ENABLED` is set to `true` on the server; otherwise this data source returns no events. " +
			"Events older than `EVENTS_DAYS_RETAIN` days are deleted, if that is set.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the organization",
				Required:            true,
			},
			"start": schema.StringAttribute{
				MarkdownDescription: "The start of the period to read events from, as an RFC 3339 timestamp, such as `2026-01-01T00:00:00Z`. " +
					"Use `timeadd(plantimestamp(), \"-24h\")` for a period relative to the time of the plan",
				Required: true,
			},
			"end": schema.StringAttribute{
				MarkdownDescription: "The end of the period, as an RFC 3339 timestamp. Defaults to the time of the read",
				Optional:            true,
				Computed:            true,
			},
			"events": schema.ListNestedAttribute{
				MarkdownDescription: "The events, newest first. IDs that do not apply to an event are `null`",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.Int64Attribute{
							MarkdownDescription: "The type of the event, as Bitwarden's event type number, such as `1300` for a created collection",
							Computed:            true,
						},
						"type_name": schema.StringAttribute{
							MarkdownDescription: "The name of the type of the event, such as `collection_created`, or the number for a type unknown to the provider",
							Computed:            true,
						},
						"date": schema.StringAttribute{
							MarkdownDescription: "When the event happened, as an RFC 3339 timestamp",
							Computed:            true,
						},
						"acting_user_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the user who caused the event",
							Computed:            true,
						},
						"user_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the user the event is about",
							Computed:            true,
						},
						"organization_user_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the organization member the event is about",
							Computed:            true,
						},
						"item_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the item the event is about",
							Computed:            true,
						},
						"collection_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the collection the event is about",
							Computed:            true,
						},
						"group_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the group the event is about",
							Computed:            true,
						},
						"policy_id": schema.StringAttribute{
							MarkdownDescription: "The ID of the policy the event is about",
							Computed:            true,
						},
						"device_type": schema.Int64Attribute{
							MarkdownDescription: "The type of the device the event came from, as Bitwarden's device type number",
							Computed:            true,
						},
						"ip_address": schema.StringAttribute{
							MarkdownDescription: "The IP address the event came from",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *OrganizationEventsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureLookupClient(req, resp); client != nil {
		d.client = client
	}
}

func (d *OrganizationEventsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrganizationEventsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	start, err := time.Parse(time.RFC3339, data.Start.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("start"), "Invalid start", "start must be an RFC 3339 timestamp: "+err.Error())
	}
	end := time.Now().UTC()
	if data.End.IsNull() {
		data.End = types.StringValue(end.Format(time.RFC3339Nano))
	} else if end, err = time.Parse(time.RFC3339, data.End.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("end"), "Invalid end", "end must be an RFC 3339 timestamp: "+err.Error())
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if end.Before(start) {
		resp.Diagnostics.AddAttributeError(path.Root("end"), "Invalid end", "end must not be before start")
		return
	}

	events, err := d.client.ListOrganizationEvents(ctx, data.OrganizationID.ValueString(), start, end)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Organization Events",
			fmt.Sprintf("Could not read the events of organization %s: %s", data.OrganizationID.ValueString(), err))
		return
	}

	data.Events = make([]OrganizationEventsModel, 0, len(events))
	for _, event := range events {
		model := OrganizationEventsModel{
			Type:               types.Int64Value(int64(event.Type)),
			TypeName:           types.StringValue(event.Type.String()),
			Date:               types.StringValue(event.Date),
			ActingUserID:       stringOrNull(event.ActingUserID),
			UserID:             stringOrNull(event.UserID),
			OrganizationUserID: stringOrNull(event.OrganizationUserID),
			ItemID:             stringOrNull(event.CipherID),
			CollectionID:       stringOrNull(event.CollectionID),
			GroupID:            stringOrNull(event.GroupID),
			PolicyID:           stringOrNull(event.PolicyID),
			DeviceType:         types.Int64Null(),
			IPAddress:          stringOrNull(event.IPAddress),
		}
		if event.DeviceType != nil {
			model.DeviceType = types.Int64Value(int64(*event.DeviceType))
		}
		data.Events = append(data.Events, model)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
