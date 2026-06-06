package nat

// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task routing-split-tunnel#T1.1: nat manager interface (AC-007)
// @sk-task ipv6-dual-stack#T2.3: add Setup6/Teardown6 (AC-003)
type Manager interface {
	Setup() error
	Setup6() error
	Teardown() error
	Teardown6() error
}
