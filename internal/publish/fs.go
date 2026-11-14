package publish

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"runtime"
	"syscall"
)

// file is the archive file while it is written.
type file interface {
	io.Writer
	Sync() error
	Close() error
}

// fileSystem holds every file system operation publishing uses, so that
// tests can inject a failure at each step of design D8.
type fileSystem interface {
	Stat(name string) (fs.FileInfo, error)
	Mkdir(name string, perm fs.FileMode) error
	CreateExclusive(name string, perm fs.FileMode) (file, error)
	SyncDir(name string) error
	Rename(oldpath, newpath string) error
	Remove(name string) error
}

type osFS struct{}

func (osFS) Stat(name string) (fs.FileInfo, error)     { return os.Stat(name) }
func (osFS) Mkdir(name string, perm fs.FileMode) error { return os.Mkdir(name, perm) }
func (osFS) Rename(oldpath, newpath string) error      { return os.Rename(oldpath, newpath) }
func (osFS) Remove(name string) error                  { return os.Remove(name) }

func (osFS) CreateExclusive(name string, perm fs.FileMode) (file, error) {
	return os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
}

// SyncDir flushes a directory where the platform supports it. Windows
// cannot open a directory for flushing, and some file systems refuse it.
// Both count as unsupported, not as failure.
func (osFS) SyncDir(name string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(name)
	if err != nil {
		return err
	}
	err = d.Sync()
	cerr := d.Close()
	if err != nil {
		if errors.Is(err, errors.ErrUnsupported) || errors.Is(err, syscall.EINVAL) {
			return nil
		}
		return err
	}
	return cerr
}
