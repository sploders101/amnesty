package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	ipcv1 "github.com/sploders101/amnesty/internal/gen/proto/com/shaunkeys/amnesty/ipc/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

type connListener struct {
	mu     sync.Mutex
	conn   net.Conn
	closed chan struct{}
	once   sync.Once
}

func newConnListener(conn net.Conn) *connListener {
	return &connListener{
		conn:   conn,
		closed: make(chan struct{}),
	}
}

func (listener *connListener) Accept() (net.Conn, error) {
	listener.mu.Lock()
	if listener.conn != nil {
		conn := listener.conn
		listener.conn = nil
		listener.mu.Unlock()
		return conn, nil
	}
	listener.mu.Unlock()

	<-listener.closed
	return nil, net.ErrClosed
}

func (listener *connListener) Close() error {
	listener.once.Do(func() {
		close(listener.closed)
	})

	listener.mu.Lock()
	conn := listener.conn
	listener.conn = nil
	listener.mu.Unlock()

	if conn != nil {
		return conn.Close()
	}
	return nil
}

func (listener *connListener) Addr() net.Addr {
	return &net.UnixAddr{Name: "anonymous", Net: "unix"}
}

type connStats struct {
	disconnected func()
}

func (gstats *connStats) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return ctx
}

func (gstats *connStats) HandleRPC(ctx context.Context, stat stats.RPCStats) {}

func (gstats *connStats) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return ctx
}

func (gstats *connStats) HandleConn(ctx context.Context, stat stats.ConnStats) {
	if _, ok := stat.(*stats.ConnEnd); ok {
		gstats.disconnected()
	}
}

func runBuildService(ctx context.Context, ipcFd int) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ipcFile := os.NewFile(uintptr(ipcFd), "grpc-server")
	ipcConn, err := net.FileConn(ipcFile)
	if err := ipcFile.Close(); err != nil {
		return err
	}
	if err != nil {
		return err
	}
	ipcListener := newConnListener(ipcConn)
	defer func() { _ = ipcListener.Close() }()

	if err := ctx.Err(); err != nil {
		return err
	}
	gstats := &connStats{
		disconnected: cancel,
	}

	server := grpc.NewServer(
		grpc.StatsHandler(gstats),
	)
	ipcv1.RegisterBuilderServiceServer(server, buildService{})
	go func() {
		<-ctx.Done()
		server.Stop()
	}()
	if err := server.Serve(ipcListener); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

type buildService struct {
	// This is fine because this is an IPC channel. If the protos are mismatched, we've got bigger problems.
	ipcv1.UnsafeBuilderServiceServer
}

func (buildService) Initialize(
	ctx context.Context,
	req *ipcv1.InitializeRequest,
) (*ipcv1.InitializeResponse, error) {
	rootDir := req.GetRootdir()

	// Remount private
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return nil, fmt.Errorf("failed to isolate container mounts: %w", err)
	}

	// Bind-mount root
	if err := syscall.Mount(rootDir, rootDir, "", syscall.MS_BIND, ""); err != nil {
		return nil, fmt.Errorf("failed to bind-mount root to itself: %w", err)
	}

	// Mount API filesystems
	if err := syscall.Mount(
		"/dev",
		filepath.Join(rootDir, "dev"),
		"",
		syscall.MS_BIND|syscall.MS_REC,
		"",
	); err != nil {
		return nil, fmt.Errorf("failed to bind mount /dev: %w", err)
	}
	if err := syscall.Mount("proc", filepath.Join(rootDir, "proc"), "proc", 0, ""); err != nil {
		return nil, fmt.Errorf("failed to mount /proc: %w", err)
	}
	if err := syscall.Mount("tmpfs", filepath.Join(rootDir, "tmp"), "tmpfs", 0, ""); err != nil {
		return nil, fmt.Errorf("failed to mount /tmp: %w", err)
	}
	if err := os.Chmod(filepath.Join(rootDir, "tmp"), 0o777); err != nil {
		return nil, fmt.Errorf("failed to chmod /tmp: %w", err)
	}
	if err := syscall.Mount("sysfs", filepath.Join(rootDir, "sys"), "sysfs", 0, ""); err != nil {
		return nil, fmt.Errorf("failed to mount /sys: %w", err)
	}

	// Pivot root
	if err := syscall.Chdir(rootDir); err != nil {
		return nil, fmt.Errorf("failed to chdir to newroot: %w", err)
	}
	if err := syscall.PivotRoot(".", "."); err != nil {
		return nil, fmt.Errorf("failed to pivot root: %w", err)
	}
	if err := syscall.Chdir("/"); err != nil {
		return nil, fmt.Errorf("failed to chdir to / after pivot: %w", err)
	}
	if err := syscall.Unmount(".", syscall.MNT_DETACH); err != nil {
		return nil, fmt.Errorf("failed to unmount oldroot: %w", err)
	}
	return ipcv1.InitializeResponse_builder{}.Build(), nil
}

func (buildService) RunCommand(
	ctx context.Context,
	req *ipcv1.RunCommandRequest,
) (*ipcv1.RunCommandResponse, error) {
	cmd := exec.CommandContext(ctx, req.GetCommand(), req.GetArgs()...)
	runas := "root"
	if req.HasRunas() {
		runas = req.GetRunas()
	}
	cmd.Env = append(buildUserEnv(runas), req.GetEnv()...)
	cred, err := getCredential(runas)
	if err != nil {
		return nil, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: cred,
	}
	output, err := cmd.CombinedOutput()
	var exitCode int32
	if err != nil {
		if err, ok := errors.AsType[*exec.ExitError](err); ok {
			exitCode = int32(err.ExitCode())
		} else {
			return nil, fmt.Errorf("failed to execute command: %w", err)
		}
	}

	return ipcv1.RunCommandResponse_builder{
		ExitCode: exitCode,
		Output:   string(output),
	}.Build(), nil
}

func (buildService) InstallCustom(
	ctx context.Context,
	req *ipcv1.InstallCustomRequest,
) (*ipcv1.InstallCustomResponse, error) {
	userCredential, err := getCredential(req.GetAsUser())
	if err != nil {
		return nil, fmt.Errorf("failed to get user credential: %w", err)
	}
	userEnv := buildUserEnv(req.GetAsUser())

	if err := os.Mkdir("/var/tmp/aurs", 0o777); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("failed to mkdir /var/tmp/aurs: %w", err)
		}
	} else {
		if err := os.Chown(
			"/var/tmp/aurs",
			int(userCredential.Uid),
			int(userCredential.Gid),
		); err != nil {
			return nil, fmt.Errorf("failed to chown dir: %w", err)
		}
	}

	pkgDir := filepath.Join("/var/tmp/aurs", req.GetPackage())
	defer func() {
		if err := os.RemoveAll(pkgDir); err != nil {
			slog.Error("Failed to clean up aur install", "package", req.GetPackage(), "error", err)
		}
	}()

	if req.HasPkgbuild() {
		if err := os.Mkdir(pkgDir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to mkdir: %w", err)
		}
		if err := os.Chown(pkgDir, int(userCredential.Uid), int(userCredential.Gid)); err != nil {
			return nil, fmt.Errorf("failed to chown build dir: %w", err)
		}
		pkgbuildPath := filepath.Join(pkgDir, "PKGBUILD")
		if err := os.WriteFile(
			pkgbuildPath,
			[]byte(req.GetPkgbuild()),
			0o644,
		); err != nil {
			return nil, fmt.Errorf("failed to write PKGBUILD: %w", err)
		}
		if err := os.Chown(
			pkgbuildPath,
			int(userCredential.Uid),
			int(userCredential.Gid),
		); err != nil {
			return nil, fmt.Errorf("failed to chown build dir: %w", err)
		}
	} else {
		cloneCmd := exec.CommandContext(
			ctx,
			"git",
			"clone",
			fmt.Sprintf("https://aur.archlinux.org/%v.git", req.GetPackage()),
			req.GetPackage(),
		)
		cloneCmd.Env = userEnv
		cloneCmd.Dir = "/var/tmp/aurs"
		cloneCmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: userCredential,
		}
		cloneOutput, err := cloneCmd.CombinedOutput()
		if err != nil {
			if err, ok := errors.AsType[*exec.ExitError](err); ok {
				return nil, fmt.Errorf(
					"failed to clone repository: %w: %v",
					err,
					string(cloneOutput),
				)
			}
			return nil, fmt.Errorf("failed to clone repository: %w", err)
		}
	}

	pkgCmd := exec.CommandContext(ctx, "makepkg", "-si", "--noconfirm")
	pkgCmd.Env = userEnv
	pkgCmd.Dir = pkgDir
	pkgCmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: userCredential,
	}
	pkgOutput, err := pkgCmd.CombinedOutput()
	if err != nil {
		if err, ok := errors.AsType[*exec.ExitError](err); ok {
			return nil, fmt.Errorf("failed to build package: %w: %v", err, string(pkgOutput))
		}
		return nil, fmt.Errorf("failed to build package: %w", err)
	}

	return ipcv1.InstallCustomResponse_builder{}.Build(), nil
}

func getCredential(username string) (*syscall.Credential, error) {
	usr, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("failed to look up user %q: %w", username, err)
	}
	uid, err := strconv.ParseUint(usr.Uid, 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(usr.Gid, 10, 32)
	if err != nil {
		return nil, err
	}
	sgidStrings, err := usr.GroupIds()
	if err != nil {
		return nil, err
	}
	var sgids []uint32
	for _, sgidString := range sgidStrings {
		sgid, err := strconv.ParseUint(sgidString, 10, 32)
		if err != nil {
			return nil, err
		}
		if sgid != gid {
			sgids = append(sgids, uint32(sgid))
		}
	}
	return &syscall.Credential{
		Uid:    uint32(uid),
		Gid:    uint32(gid),
		Groups: sgids,
	}, nil
}

func buildUserEnv(username string) []string {
	home := filepath.Join("/home", username)
	return []string{
		"HOME=" + home,
		"USER=" + username,
		"LOGNAME=" + username,
		"PATH=/usr/local/bin:/usr/bin",
		"SHELL=/bin/bash",
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local/share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local/state"),
	}
}
