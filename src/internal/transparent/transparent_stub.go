//go:build !linux

package transparent

// @sk-task transparent-proxy#T2.1: stub manager for non-Linux (DEC-001)
func newManager() TransparentManager {
	return &noopManager{}
}
