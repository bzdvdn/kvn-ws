// @sk-task foundation#T1.3: internal stubs (AC-002)

package auth

// @sk-task core-tunnel-mvp#T3.2: bearer-token auth (AC-006)
func ValidateToken(token string, validTokens []string) bool {
	for _, t := range validTokens {
		if t == token {
			return true
		}
	}
	return false
}
