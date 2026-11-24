package authz

var LogsReadRedacted = Permission{Resource: "logs", Action: "read_redacted"}

func init() {
	RegisterResource(ResourceDefinition{
		Resource: "logs", LabelKey: "Usage logs",
		Actions: []ActionDefinition{{
			Action: "read_redacted", LabelKey: "Read redacted usage logs",
			DescriptionKey: "Read ordinary users' usage logs with a bot system access token.",
			DefaultRoles:   []string{BuiltInRoleBot},
		}},
	})
}
