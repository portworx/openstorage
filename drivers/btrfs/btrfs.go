package btrfs

import (
	"fmt"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"time"

	graph "github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/daemon/graphdriver/btrfs"

	"github.com/libopenstorage/kvdb"
	"github.com/libopenstorage/openstorage/api"
	"github.com/libopenstorage/openstorage/pkg/chaos"
	"github.com/libopenstorage/openstorage/volume"
)

const (
	Name      = "btrfs"
	RootParam = "home"
	Volumes   = "volumes"
)

var (
	koStrayCreate chaos.ID
	koStrayDelete chaos.ID
)

type btrfsDriver struct {
	*volume.DefaultBlockDriver
	*volume.DefaultEnumerator
	btrfs graph.Driver
	root  string
}

func uuid() (string, error) {
	out, err := exec.Command("uuidgen").Output()
	if err != nil {
		return "", err
	}
	id := string(out)
	id = strings.TrimSuffix(id, "\n")
	return id, nil
}

func Init(params volume.DriverParams) (volume.VolumeDriver, error) {
	root, ok := params[RootParam]
	if !ok {
		return nil, fmt.Errorf("Root directory should be specified with key %q", RootParam)
	}
	home := path.Join(root, Volumes)
	d, err := btrfs.Init(home, nil)
	if err != nil {
		return nil, err
	}
	s := volume.NewDefaultEnumerator(Name, kvdb.Instance())
	return &btrfsDriver{btrfs: d, root: root, DefaultEnumerator: s}, nil
}

func (d *btrfsDriver) String() string {
	return Name
}

// Status diagnostic information
func (d *btrfsDriver) Status() [][2]string {
	return d.btrfs.Status()
}

// Create a new subvolume. The volume spec is not taken into account.
func (d *btrfsDriver) Create(locator api.VolumeLocator,
	options *api.CreateOptions,
	spec *api.VolumeSpec) (api.VolumeID, error) {

	if spec.Format != api.FsBtrfs && spec.Format != "" {
		return api.BadVolumeID, fmt.Errorf("Filesystem format (%v) must be %v",
			spec.Format, api.FsBtrfs)
	}

	volumeID, err := uuid()
	if err != nil {
		return api.BadVolumeID, err
	}

	v := &api.Volume{
		ID:       api.VolumeID(volumeID),
		Locator:  locator,
		Ctime:    time.Now(),
		Spec:     spec,
		LastScan: time.Now(),
		Format:   api.FsBtrfs,
		State:    api.VolumeAvailable,
	}
	err = d.CreateVol(v)
	if err != nil {
		return api.BadVolumeID, err
	}
	err = d.btrfs.Create(volumeID, "")
	if err != nil {
		return api.BadVolumeID, err
	}
	v.DevicePath, err = d.btrfs.Get(volumeID, "")
	if err != nil {
		return v.ID, err
	}
	err = d.UpdateVol(v)
	return v.ID, err
}

// Delete subvolume
func (d *btrfsDriver) Delete(volumeID api.VolumeID) error {
	err := d.DeleteVol(volumeID)
	chaos.Now(koStrayDelete)
	if err == nil {
		err = d.btrfs.Remove(string(volumeID))
	}
	return err
}

// Mount bind mount btrfs subvolume
func (d *btrfsDriver) Mount(volumeID api.VolumeID, mountpath string) error {
	v, err := d.GetVol(volumeID)
	if err != nil {
		return err
	}
	err = syscall.Mount(v.DevicePath,
		mountpath,
		string(v.Format),
		syscall.MS_BIND, "")
	if err != nil {
		return fmt.Errorf("Faield to mount %v at %v: %v", v.DevicePath, mountpath, err)
	}
	v.AttachPath = mountpath
	err = d.UpdateVol(v)
	return err
}

// Unmount btrfs subvolume
func (d *btrfsDriver) Unmount(volumeID api.VolumeID, mountpath string) error {
	v, err := d.GetVol(volumeID)
	if err != nil {
		return err
	}
	if v.AttachPath == "" {
		return fmt.Errorf("Device %v not mounted", volumeID)
	}
	err = syscall.Unmount(v.AttachPath, 0)
	if err != nil {
		return err
	}
	v.AttachPath = ""
	err = d.UpdateVol(v)
	return err
}

// Snapshot create new subvolume from volume
func (d *btrfsDriver) Snapshot(volumeID api.VolumeID, labels api.Labels) (api.SnapID, error) {
	snapID, err := uuid()
	if err != nil {
		return api.BadSnapID, err
	}

	snap := &api.VolumeSnap{
		ID:         api.SnapID(snapID),
		VolumeID:   volumeID,
		SnapLabels: labels,
		Ctime:      time.Now(),
	}
	err = d.CreateSnap(snap)
	if err != nil {
		return api.BadSnapID, err
	}
	chaos.Now(koStrayCreate)
	err = d.btrfs.Create(snapID, string(volumeID))
	if err != nil {
		return api.BadSnapID, err
	}
	return snap.ID, nil
}

// SnapDelete Delete subvolume
func (d *btrfsDriver) SnapDelete(snapID api.SnapID) error {
	err := d.DeleteSnap(snapID)
	chaos.Now(koStrayDelete)
	if err == nil {
		err = d.btrfs.Remove(string(snapID))
	}
	return err
}

// Stats for specified volume.
func (d *btrfsDriver) Stats(volumeID api.VolumeID) (api.VolumeStats, error) {
	return api.VolumeStats{}, nil
}

// Alerts on this volume.
func (d *btrfsDriver) Alerts(volumeID api.VolumeID) (api.VolumeAlerts, error) {
	return api.VolumeAlerts{}, nil
}

// Shutdown and cleanup.
func (d *btrfsDriver) Shutdown() {
}

func init() {
	volume.Register(Name, volume.File, Init)
}
