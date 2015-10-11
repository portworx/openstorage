// +build linux

package mount

import (
	"regexp"

	"github.com/docker/docker/pkg/mount"
)

// NFSMounter implements Manager and keeps track of active mounts for volume drivers.
type NFSMounter struct {
	server string
	Mounter
}

// NewNFSMounter instance
func NewNFSMounter(server string) (Manager, error) {
	m := &NFSMounter{
		server:  server,
		Mounter: Mounter{mounts: make(DeviceMap)},
	}
	err := m.Load("")
	if err != nil {
		return nil, err
	}
	return m, nil
}

// Load mount table
func (m *NFSMounter) Load(source string) error {
	info, err := mount.GetMounts()
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`,addr=(.*)`)
MountLoop:
	for _, v := range info {
		if m.server != "" {
			if v.Fstype != "nfs" {
				continue
			}
			matches := re.FindStringSubmatch(v.VfsOpts)
			if len(matches) != 2 {
				continue
			}
			if matches[1] != m.server {
				continue
			}
		}
		mount, ok := m.mounts[v.Source]
		if !ok {
			mount = &Info{
				Device:     v.Source,
				Fs:         v.Fstype,
				Minor:      v.Minor,
				Mountpoint: make([]PathInfo, 0),
			}
			m.mounts[v.Source] = mount
		}
		// Allow Load to be called multiple times.
		for _, p := range mount.Mountpoint {
			if p.Path == v.Mountpoint {
				continue MountLoop
			}
		}
		// XXX Reconstruct refs.
		mount.Mountpoint = append(mount.Mountpoint, PathInfo{Path: v.Mountpoint, ref: 1})
	}
	return nil
}
