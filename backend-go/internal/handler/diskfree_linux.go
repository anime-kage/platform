//go:build linux

package handler

import "syscall"

// diskUsage reports the filesystem holding path. Blocks are counted with
// Bavail, not Bfree: Bfree includes the root-reserved blocks that an ordinary
// upload can never touch, which would overstate the space translators have.
func diskUsage(path string) (total, free uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bs := uint64(st.Bsize)
	return st.Blocks * bs, st.Bavail * bs, nil
}
