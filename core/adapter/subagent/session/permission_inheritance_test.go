package session

import (
	"context"
	"testing"

	memoryadapter "myai/core/adapter/session/memory"
	toolcommand "myai/core/application/tool/command"
	toolservice "myai/core/application/tool/service"
	domainsubagent "myai/core/domain/subagent"
	domainsession "myai/core/session"
	tooldef "myai/core/tool/tool"
)

func TestFactoryIntersectsParentCapabilitiesAndKeepsApproval(t *testing.T) {
	cases := []struct {
		name           string
		parent         domainsession.PermissionMode
		child          domainsubagent.CapabilityMode
		want           domainsession.PermissionMode
		write, execute bool
	}{
		{"readonly parent", domainsession.PermissionModeReadonly, domainsubagent.CapabilityModeAll, domainsession.PermissionModeReadonly, false, false},
		{"write parent execute child", domainsession.PermissionModeReadWrite, domainsubagent.CapabilityModeExecute, domainsession.PermissionModeReadonly, false, false},
		{"execute parent write child", domainsession.PermissionModeExecute, domainsubagent.CapabilityModeReadWrite, domainsession.PermissionModeReadonly, false, false},
		{"ask write child", domainsession.PermissionModeAsk, domainsubagent.CapabilityModeReadWrite, domainsession.PermissionModeAsk, true, false},
		{"ask execute child", domainsession.PermissionModeAsk, domainsubagent.CapabilityModeExecute, domainsession.PermissionModeAsk, false, true},
		{"full write child", domainsession.PermissionModeFull, domainsubagent.CapabilityModeReadWrite, domainsession.PermissionModeReadWrite, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			memory := memoryadapter.NewStore("model-1")
			if err := memory.PutSessionState(domainsession.InitialState{ID: "parent", PermissionMode: c.parent, AllowedTools: []string{"read_file", "write_file"}, EnforceToolAllowlist: true}, false); err != nil {
				t.Fatal(err)
			}
			request := childSessionRequest("child")
			request.Definition.CapabilityMode = c.child
			request.Definition.AllowedTools = []string{"read_file", "write_file", "shell"}
			child, err := (Factory{Memory: memory}).Create(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if child.PermissionMode != c.want || tooldef.PermissionAllowsCeiling(tooldef.PermissionWrite, child.PermissionCeiling) != c.write || tooldef.PermissionAllowsCeiling(tooldef.PermissionExecute, child.PermissionCeiling) != c.execute {
				t.Fatalf("wrong intersection: mode=%s ceiling=%v", child.PermissionMode, child.PermissionCeiling)
			}
			if len(child.AllowedTools) != 2 || !child.EnforceToolAllowlist {
				t.Fatalf("parent tool allowlist lost: %v", child.AllowedTools)
			}
			if c.parent == domainsession.PermissionModeAsk {
				permission, name := tooldef.PermissionWrite, "write_file"
				if c.execute {
					permission, name = tooldef.PermissionExecute, "extension"
				}
				decision := (toolservice.PermissionService{}).Allow(toolcommand.Permission{Name: name, Permission: permission, Mode: child.PermissionMode})
				if decision.Allowed {
					t.Fatal("parent ask requirement was bypassed")
				}
			}
		})
	}
}

func TestFactoryReuseTightensLiveStoreAndNeverWidensEmptyCeiling(t *testing.T) {
	memory := memoryadapter.NewStore("model-1")
	for _, state := range []domainsession.InitialState{
		{ID: "parent", PermissionMode: domainsession.PermissionModeReadonly, PermissionCeiling: []tooldef.Permission{tooldef.PermissionRead}},
		{ID: "child", ParentSessionID: "parent", PermissionMode: domainsession.PermissionModeFull},
	} {
		if err := memory.PutSessionState(state, false); err != nil {
			t.Fatal(err)
		}
	}
	factory := Factory{Memory: memory}
	request := childSessionRequest("child")
	request.Definition.CapabilityMode = domainsubagent.CapabilityModeAll
	request.Definition.AllowedTools = []string{"read_file", "write_file"}
	request.WorkspaceRoot = "new-root"
	child, err := factory.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	live, _ := memory.Snapshot("child")
	if child.PermissionMode != domainsession.PermissionModeReadonly || live.PermissionMode != domainsession.PermissionModeReadonly || live.WorkspaceRoot != "new-root" || len(live.PermissionCeiling) != 1 {
		t.Fatalf("live store retained excessive child privileges: mode=%s ceiling=%v root=%s", live.PermissionMode, live.PermissionCeiling, live.WorkspaceRoot)
	}
	if err := memory.SetPermissionCeilingForSession("child", []tooldef.Permission{}); err != nil {
		t.Fatal(err)
	}
	if err := memory.SetPermissionModeForSession("parent", domainsession.PermissionModeFull); err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	live, _ = memory.Snapshot("child")
	if live.PermissionCeiling == nil || len(live.PermissionCeiling) != 0 {
		t.Fatalf("deny-all ceiling widened: %v", live.PermissionCeiling)
	}
}

func TestFactoryRejectsUnavailableOrDifferentParent(t *testing.T) {
	memory := memoryadapter.NewStore("model-1")
	factory := Factory{Memory: memory}
	if _, err := factory.Create(context.Background(), childSessionRequest("child")); err == nil {
		t.Fatal("missing parent was accepted")
	}
	if err := memory.PutSessionState(domainsession.InitialState{ID: "parent", PermissionMode: domainsession.PermissionModeFull}, false); err != nil {
		t.Fatal(err)
	}
	if err := memory.PutSessionState(domainsession.InitialState{ID: "child", ParentSessionID: "other"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Create(context.Background(), childSessionRequest("child")); err == nil {
		t.Fatal("child was reparented")
	}
}
