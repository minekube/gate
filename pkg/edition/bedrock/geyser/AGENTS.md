# pkg/edition/bedrock/geyser agent notes

Bedrock gamertags cross into Java profiles in `pkg/edition/bedrock/geyser/geyser.go`; the rendered `usernameFormat` must be normalized to Java's 16-character ASCII username alphabet there. Modern Paper rejects the traditional dot prefix and Bedrock spaces before login, so preserve `javaCompatibleUsername` and `username_test.go` when changing profile construction.
