// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task security-acl#T1: updated for TokenCfg

package auth

import "github.com/bzdvdn/kvn-ws/src/internal/config"

// @sk-task core-tunnel-mvp#T3.2: bearer-token auth (AC-006)
// @sk-task security-acl#T1: supports both string slice and TokenCfg
func ValidateToken(token string, validTokens []string) bool {
	for _, t := range validTokens {
		if t == token {
			return true
		}
	}
	return false
}

// @sk-task security-acl#T1: FindToken looks up a token in TokenCfg slice
func FindToken(secret string, tokens []config.TokenCfg) *config.TokenCfg {
	for i, t := range tokens {
		if t.Secret == secret {
			return &tokens[i]
		}
	}
	return nil
}
