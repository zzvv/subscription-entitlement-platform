package domain

type Entity struct{ ID, Tenant, Scope, Plan, Status string }
type Command struct{ ID, Tenant, Scope, Action, Plan string }

func NewCommand(id, tenant, scope, action string) Command {
	return Command{ID: id, Tenant: tenant, Scope: scope, Action: action, Plan: "standard"}
}
func (c Command) Valid() bool { return c.ID != "" && c.Tenant != "" && c.Scope != "" }
func NewEntity(id, tenant, scope string) Entity {
	return Entity{ID: id, Tenant: tenant, Scope: scope, Plan: "standard", Status: "active"}
}
