package transport

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jlaffaye/ftp"
)

type FTPClient struct {
	host     string
	port     string
	user     string
	password string
	rootPath string
}

func NewFTP(cfg map[string]string) (*FTPClient, error) {
	return &FTPClient{
		host:     cfg["host"],
		port:     cfg["port"],
		user:     cfg["user"],
		password: cfg["password"],
		rootPath: cfg["remote_path"],
	}, nil
}

func (f *FTPClient) Name() string { return "ftp" }

func (f *FTPClient) connect() (*ftp.ServerConn, error) {
	addr := f.host + ":" + f.port
	c, err := ftp.Dial(addr, ftp.DialWithTimeout(10*time.Second))
	if err != nil {
		return nil, fmt.Errorf("ftp dial: %w", err)
	}
	if err := c.Login(f.user, f.password); err != nil {
		c.Quit()
		return nil, fmt.Errorf("ftp login: %w", err)
	}
	return c, nil
}

func (f *FTPClient) List(remotePath string) ([]FileInfo, error) {
	c, err := f.connect()
	if err != nil {
		return nil, err
	}
	defer c.Quit()

	path := filepath.Join(f.rootPath, remotePath)
	entries, err := c.List(path)
	if err != nil {
		return nil, fmt.Errorf("ftp list: %w", err)
	}

	var result []FileInfo
	for _, e := range entries {
		result = append(result, FileInfo{
			Name:    e.Name,
			Path:    filepath.Join(remotePath, e.Name),
			Size:    int64(e.Size),
			ModTime: e.Time,
			IsDir:   e.Type == ftp.EntryTypeFolder,
		})
	}
	return result, nil
}

func (f *FTPClient) Push(localPath, remotePath string) error {
	c, err := f.connect()
	if err != nil {
		return err
	}
	defer c.Quit()

	fullRemote := filepath.Join(f.rootPath, remotePath)
	if err := f.mkdirAll(c, filepath.Dir(fullRemote)); err != nil {
		return err
	}

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("ftp push open: %w", err)
	}
	defer file.Close()

	if err := c.Stor(fullRemote, file); err != nil {
		return fmt.Errorf("ftp stor: %w", err)
	}
	return nil
}

func (f *FTPClient) Pull(remotePath, localPath string) error {
	c, err := f.connect()
	if err != nil {
		return err
	}
	defer c.Quit()

	fullRemote := filepath.Join(f.rootPath, remotePath)
	r, err := c.Retr(fullRemote)
	if err != nil {
		return fmt.Errorf("ftp retr: %w", err)
	}
	defer r.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, r)
	return err
}

func (f *FTPClient) Delete(remotePath string) error {
	c, err := f.connect()
	if err != nil {
		return err
	}
	defer c.Quit()
	return c.Delete(filepath.Join(f.rootPath, remotePath))
}

func (f *FTPClient) Test() error {
	c, err := f.connect()
	if err != nil {
		return err
	}
	c.Quit()
	return nil
}

func (f *FTPClient) mkdirAll(c *ftp.ServerConn, dir string) error {
	if dir == "." || dir == "/" {
		return nil
	}
	parent := filepath.Dir(dir)
	if parent != dir {
		f.mkdirAll(c, parent)
	}
	c.MakeDir(dir)
	return nil
}
