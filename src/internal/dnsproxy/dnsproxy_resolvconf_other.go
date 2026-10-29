//go:build !linux

package dnsproxy

// ResolvConfBackup is a no-op on platforms where KVN does not rewrite the
// system resolver configuration. The DNS proxy still runs and handles
// explicit direct-domain queries; system DNS remains owned by the OS/other VPN.
type ResolvConfBackup struct{}

func BackupResolvConf() (*ResolvConfBackup, error) { return nil, nil }

func OverrideResolvConf(string) error { return nil }

func (b *ResolvConfBackup) Restore() error { return nil }
