package config

import "os"

type SystemConfig struct {
	
}

type OSConfig struct {
	Version  string   `yaml:"version"`
	Kind     string   `yaml:"kind"`
	Name     string   `yaml:"name"`
	Modules  []string `yaml:"modules"`
	Username string   `yaml:"username"`
	Locale   string   `yaml:"locale"`
	Timezone string   `yaml:"timezone"`
	Mounts   []Mount  `yaml:"mounts"`
}

// Describes a single mountpoint
type Mount struct {
	Mountpoint TemplatedString `yaml:"mountpoint"`
	Source     MountSource     `yaml:"source"`
}

// Indicates the backing storage of the mountpoint
type MountSource struct {
	PVC  MountSourcePVC  `yaml:"pvc"`
	Disk MountSourceDisk `yaml:"disk"`
}

// Describes a persistent volume claim that should be preserved
// across rebuilds. Stored within the same btrfs system as the
// OS which reserved it. This volume is local to the btrfs
// filesystem in which the OS is stored. Therefore, if multiple
// btrfs partitions exist (for example, to isolate different use
// cases of the same system by way of LUKS encryption), a PVC may
// not be shared between them.
type MountSourcePVC struct {
	Name TemplatedString `yaml:"name"`
}

// Describes a manual disk reference
type MountSourceDisk struct {
	UUID    TemplatedString `yaml:"uuid"`
	Path    TemplatedString `yaml:"path"`
	FSType  TemplatedString `yaml:"fs_type"`
	Options TemplatedString `yaml:"options"`
	Dump    int             `yaml:"dump"`
	Pass    int             `yaml:"pass"`
}

type OSModule struct {
	Version         string            `yaml:"version"`
	Kind            string            `yaml:"kind"`
	Name            string            `yaml:"name"`
	Description     string            `yaml:"description"`
	Packages        []TemplatedString `yaml:"packages"`
	AURPackages     []TemplatedString `yaml:"aur_packages"`
	FlatpakPackages []TemplatedString `yaml:"flatpak_packages"`
	FlatpakRemotes  []FlatpakRemote   `yaml:"flatpak_remotes"`
	Services        Services          `yaml:"services"`
	Polkit          []PolkitEntry     `yaml:"polkit"`
	InstallFiles    []InstallFile     `yaml:"install_files"`
	AdminGroups     []TemplatedString `yaml:"admin_groups"`
	UserGroups      []TemplatedString `yaml:"user_groups"`
	Postinstall     []Postinstall     `yaml:"postinstall"`
}

type FlatpakRemote struct {
	Name string          `yaml:"name"`
	Url  TemplatedString `yaml:"url"`
}

type Services struct {
	System []TemplatedString `yaml:"system"`
	User   []TemplatedString `yaml:"user"`
}

type PolkitEntry struct {
	Id     string          `yaml:"id"`
	Result TemplatedString `yaml:"result"`
}

type InstallFile struct {
	Path     TemplatedString `yaml:"path"`
	Mode     os.FileMode     `yaml:"mode"`
	Contents TemplatedString `yaml:"contents"`
}

type Postinstall struct {
	Runas     TemplatedString `yaml:"runas"`
	AllowSudo bool            `yaml:"allow_sudo"`
	Cmd       TemplatedString `yaml:"cmd"`
}
