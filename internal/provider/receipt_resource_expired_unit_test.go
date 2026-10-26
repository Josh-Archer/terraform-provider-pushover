// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// expiredReceiptServer mimics Pushover once a receipt is no longer queryable.
func expiredReceiptServer(t *testing.T, cancelCalls *int32) (*ReceiptResource, schema.Schema) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/cancel") {
			atomic.AddInt32(cancelCalls, 1)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":0,"request":"req-x","errors":["receipt not found; may be invalid or expired"]}`))
	}))
	t.Cleanup(srv.Close)
	res := &ReceiptResource{client: pushover.NewClientWithBase("tok", srv.URL, srv.Client())}
	sr := &resource.SchemaResponse{}
	res.Schema(context.Background(), resource.SchemaRequest{}, sr)
	return res, sr.Schema
}

func knownReceiptModel(cancel bool) ReceiptResourceModel {
	return ReceiptResourceModel{
		Receipt:              types.StringValue("rcpt1"),
		APIToken:             types.StringNull(),
		CancelOnDestroy:      types.BoolValue(cancel),
		ID:                   types.StringValue("rcpt1"),
		Acknowledged:         types.BoolValue(false),
		AcknowledgedAt:       types.Int64Value(0),
		AcknowledgedBy:       types.StringValue(""),
		AcknowledgedByDevice: types.StringValue(""),
		LastDeliveredAt:      types.Int64Value(1700000010),
		Expired:              types.BoolValue(false),
		ExpiresAt:            types.Int64Value(1700003600),
		CalledBack:           types.BoolValue(false),
		CalledBackAt:         types.Int64Value(0),
		RequestID:            types.StringValue("req-old"),
	}
}

func receiptState(t *testing.T, s schema.Schema, m ReceiptResourceModel) tfsdk.State {
	t.Helper()
	st := tfsdk.State{Schema: s}
	if d := st.Set(context.Background(), &m); d.HasError() {
		t.Fatalf("state set: %v", d)
	}
	return st
}

func receiptPlan(t *testing.T, s schema.Schema, m ReceiptResourceModel) tfsdk.Plan {
	t.Helper()
	p := tfsdk.Plan{Schema: s}
	if d := p.Set(context.Background(), &m); d.HasError() {
		t.Fatalf("plan set: %v", d)
	}
	return p
}

func TestReceiptResource_Read_ExpiredKeepsStateMarkedExpired(t *testing.T) {
	t.Parallel()
	var cancels int32
	res, s := expiredReceiptServer(t, &cancels)
	st := receiptState(t, s, knownReceiptModel(true))
	resp := &resource.ReadResponse{State: st}
	res.Read(context.Background(), resource.ReadRequest{State: st}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("Read removed the resource from state; expired receipts must be retained")
	}
	var got ReceiptResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("get state: %v", d)
	}
	if !got.Expired.ValueBool() {
		t.Error("expected expired = true")
	}
	if got.LastDeliveredAt.ValueInt64() != 1700000010 || got.ExpiresAt.ValueInt64() != 1700003600 || got.RequestID.ValueString() != "req-old" {
		t.Errorf("last-known values not retained: %+v", got)
	}
}

func TestReceiptResource_Create_ExpiredSucceedsMarkedExpired(t *testing.T) {
	t.Parallel()
	var cancels int32
	res, s := expiredReceiptServer(t, &cancels)
	m := knownReceiptModel(true)
	m.ID = types.StringUnknown()
	m.Acknowledged, m.Expired, m.CalledBack = types.BoolUnknown(), types.BoolUnknown(), types.BoolUnknown()
	m.AcknowledgedAt, m.LastDeliveredAt, m.ExpiresAt, m.CalledBackAt = types.Int64Unknown(), types.Int64Unknown(), types.Int64Unknown(), types.Int64Unknown()
	m.AcknowledgedBy, m.AcknowledgedByDevice, m.RequestID = types.StringUnknown(), types.StringUnknown(), types.StringUnknown()
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s}}
	res.Create(context.Background(), resource.CreateRequest{Plan: receiptPlan(t, s, m)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create must not fail for an expired receipt: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 {
		t.Errorf("expected one warning, got %v", resp.Diagnostics)
	}
	var got ReceiptResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("get state: %v", d)
	}
	if got.ID.ValueString() != "rcpt1" || !got.Expired.ValueBool() || got.ExpiresAt.IsUnknown() || got.RequestID.IsUnknown() {
		t.Errorf("unexpected state after create: %+v", got)
	}
}

func TestReceiptResource_Update_ExpiredAppliesPlanAndKeepsState(t *testing.T) {
	t.Parallel()
	var cancels int32
	res, s := expiredReceiptServer(t, &cancels)
	prior := receiptState(t, s, knownReceiptModel(true))
	planned := knownReceiptModel(false)
	planned.Acknowledged, planned.Expired = types.BoolUnknown(), types.BoolUnknown()
	planned.ExpiresAt, planned.LastDeliveredAt = types.Int64Unknown(), types.Int64Unknown()
	planned.RequestID = types.StringUnknown()
	resp := &resource.UpdateResponse{State: prior}
	res.Update(context.Background(), resource.UpdateRequest{Plan: receiptPlan(t, s, planned), State: prior}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("Update dropped the resource from state")
	}
	var got ReceiptResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("get state: %v", d)
	}
	if got.CancelOnDestroy.ValueBool() {
		t.Error("planned cancel_on_destroy = false was not applied")
	}
	if !got.Expired.ValueBool() || got.ExpiresAt.ValueInt64() != 1700003600 || got.LastDeliveredAt.ValueInt64() != 1700000010 || got.RequestID.ValueString() != "req-old" {
		t.Errorf("unexpected state after update: %+v", got)
	}
}

func TestReceiptResource_Delete_ExpiredSkipsCancel(t *testing.T) {
	t.Parallel()
	var cancels int32
	res, s := expiredReceiptServer(t, &cancels)
	m := knownReceiptModel(true)
	m.Expired = types.BoolValue(true)
	st := receiptState(t, s, m)
	resp := &resource.DeleteResponse{State: st}
	res.Delete(context.Background(), resource.DeleteRequest{State: st}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %v", resp.Diagnostics)
	}
	if atomic.LoadInt32(&cancels) != 0 {
		t.Error("cancel API should not be called for an expired receipt")
	}
}
