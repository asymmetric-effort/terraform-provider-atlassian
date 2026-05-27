// Package unit contains tests for the nil ProviderData path in Configure.
package unit

import (
	"context"
	"testing"

	groupdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/group"
	roledatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/role"
	userds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/user"
	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	tokenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/token"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestGroupResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := groupresource.NewResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestGroupMembershipConfigureNil(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestRoleResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := roleresource.NewResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestRoleAssignmentConfigureNil(t *testing.T) {
	t.Parallel()
	r := roleresource.NewAssignmentResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestTokenResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := tokenresource.NewResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestUserDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := userds.NewDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestGroupDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := groupdatasource.NewDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestRoleDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := roledatasource.NewDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}
