// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task routing-split-tunnel#T1.1: nat manager interface (AC-007)
package nat

// @sk-task routing-split-tunnel#T1.1: nat manager interface (AC-007)
type Manager interface {
	Setup() error
	Teardown() error
}
