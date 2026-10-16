package authz

const ResourceFinance = "finance"

var (
	FinanceRead   = Permission{Resource: ResourceFinance, Action: ActionRead}
	FinanceExport = Permission{Resource: ResourceFinance, Action: "export"}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceFinance,
		LabelKey: "Finance",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View financial records", DescriptionKey: "View financial records", DefaultRoles: []string{BuiltInRoleAdmin, BuiltInRoleFinance}},
			{Action: "export", LabelKey: "Export financial records", DescriptionKey: "Export financial records", DefaultRoles: []string{BuiltInRoleAdmin, BuiltInRoleFinance}},
		},
	})
}
