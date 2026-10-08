package bedrockidentity

// Policy controls which Bedrock principals may join a Connect endpoint.
type Policy string

const (
	// PolicyLinkedJavaOnly keeps the existing protected-endpoint behavior:
	// Bedrock players must resolve to a verified linked Java account.
	PolicyLinkedJavaOnly Policy = "linked_java_only"

	// PolicyTrustedBedrockXUID lets Connect Edge trust the Microsoft/Xbox XUID
	// as the Bedrock principal.
	PolicyTrustedBedrockXUID Policy = "trusted_bedrock_xuid"
)

func (p Policy) Valid() bool {
	switch p {
	case PolicyLinkedJavaOnly, PolicyTrustedBedrockXUID:
		return true
	default:
		return false
	}
}
