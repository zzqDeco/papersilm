package protocol

import (
	"encoding/json"
	"testing"
)

func TestApprovalRequestJSONKeepsLegacyFieldsWithPermissionRequests(t *testing.T) {
	t.Parallel()

	approval := ApprovalRequest{
		PlanID:          "plan_1",
		CheckpointID:    "checkpoint_1",
		PendingNodeIDs:  []string{"node_1"},
		Mode:            "tool",
		ActiveRequestID: "req_1",
		Requests: []PermissionRequest{
			{
				RequestID: "req_1",
				Tool:      "workspace_command",
				Question:  "Run command?",
				Options: []PermissionOption{
					{Value: "accept-once", Label: "Yes"},
				},
			},
		},
	}
	data, err := json.Marshal(approval)
	if err != nil {
		t.Fatalf("marshal approval: %v", err)
	}
	var decoded ApprovalRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal approval: %v", err)
	}
	if decoded.PlanID != "plan_1" || decoded.CheckpointID != "checkpoint_1" {
		t.Fatalf("legacy fields were not preserved: %+v", decoded)
	}
	if decoded.ActiveRequestID != "req_1" || len(decoded.Requests) != 1 || decoded.Requests[0].Question != "Run command?" {
		t.Fatalf("permission request fields were not preserved: %+v", decoded)
	}
}
