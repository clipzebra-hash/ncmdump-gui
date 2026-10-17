package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"git.taurusxin.com/taurusxin/ncmdump-go/ncmcrypt"
	"git.taurusxin.com/taurusxin/ncmdump-gui/utils"
)

// App struct
type App struct {
	started  time.Time
	ctx      context.Context
	mu       sync.Mutex
	cancel   context.CancelFunc
	progress BatchProgress
	emit     func(string, ...interface{})
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emit = func(name string, args ...interface{}) { runtime.EventsEmit(ctx, name, args...) }
}

// select files to dump
func (a *App) SelectFiles() []string {
	selection, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "请选择文件",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "NCM Files",
				Pattern:     "*.ncm",
			},
		},
	})
	if err != nil {
		return []string{}
	}
	return selection
}

func (a *App) SelectFolder() string {
	folder, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "请选择保存目录",
	})
	if err != nil {
		return ""
	}
	return folder
}

func (a *App) SelectFilesFromFolder(ext string) []string {
	folder, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "请选择文件夹",
	})
	if err != nil {
		return []string{}
	} else {
		files, err := utils.ListFilesFromFolder(folder, ext)
		if err != nil {
			return []string{}
		}
		return files
	}
}

type Status = string

const (
	Pending    Status = "pending"
	Processing Status = "processing"
	Done       Status = "done"
	Error      Status = "error"
)

type NcmFile struct {
	Name   string
	Status Status
}

// At most four workers; progress is protected by mu and counts terminal files only.
type FileFailure struct {
	Index         int
	File, Message string
}
type BatchProgress struct {
	Deleted                           int
	Errors                            []FileFailure
	Running                           bool
	Total, Completed, Success, Failed int
	Current, Warning, Error           string
	Elapsed                           float64
	Cancelled                         bool
}

func (a *App) GetProgress() BatchProgress {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.progress
	p.Errors = append([]FileFailure(nil), a.progress.Errors...)
	if p.Running {
		p.Elapsed = time.Since(a.started).Seconds()
	}
	return p
}
func (a *App) CancelBatch() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}
func (a *App) publish() {
	a.mu.Lock()
	p := a.progress
	p.Errors = append([]FileFailure(nil), a.progress.Errors...)
	a.mu.Unlock()
	if a.emit != nil {
		a.emit("batch-progress", p)
	}
}
func (a *App) ProcessFiles(files []NcmFile, savePath string, deleteSource bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.progress.Running {
		return fmt.Errorf("已有转换任务正在运行")
	}
	type job struct {
		path    string
		indices []int
	}
	jobs := []job{}
	seen := map[string]int{}
	for i, f := range files {
		if f.Status != Pending {
			continue
		}
		if deleteSource {
			info, err := os.Lstat(f.Name)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("删除源文件选项只支持普通 NCM 文件（不支持符号链接）: %s", f.Name)
			}
		}
		p, e := filepath.Abs(f.Name)
		if e != nil {
			return e
		}
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		if !strings.EqualFold(filepath.Ext(p), ".ncm") {
			return fmt.Errorf("仅支持 NCM 文件")
		}
		if existing, ok := seen[p]; ok {
			jobs[existing].indices = append(jobs[existing].indices, i)
		} else {
			seen[p] = len(jobs)
			jobs = append(jobs, job{p, []int{i}})
		}
	}
	if len(jobs) == 0 {
		return fmt.Errorf("没有待处理文件")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.started = time.Now()
	a.progress = BatchProgress{Running: true, Total: len(jobs)}
	go func() {
		started := time.Now()
		defer cancel()

		active := map[int]string{}
		refreshCurrent := func() {
			keys := []int{}
			for k := range active {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			names := []string{}
			for _, k := range keys {
				names = append(names, active[k])
			}
			a.progress.Current = strings.Join(names, " · ")
		}
		queue := make(chan job)
		var workers sync.WaitGroup
		for w := 0; w < 4; w++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for j := range queue {
					if ctx.Err() != nil {
						continue
					}
					a.mu.Lock()
					active[j.indices[0]] = j.path
					refreshCurrent()
					a.mu.Unlock()
					a.publish()
					if a.emit != nil {
						for _, index := range j.indices {
							a.emit("file-status-changed", index, Processing)
						}
					}
					_, err := convertFileWithOptions(ctx, j.path, savePath, conversionOptions{DeleteSource: deleteSource})
					status := Done
					a.mu.Lock()
					delete(active, j.indices[0])
					refreshCurrent()
					if errors.Is(err, context.Canceled) {
						status = Pending
					} else {
						a.progress.Completed++
						if err != nil {
							status = Error
							a.progress.Failed++
							a.progress.Error = err.Error()
							for _, index := range j.indices {
								a.progress.Errors = append(a.progress.Errors, FileFailure{index, j.path, err.Error()})
							}
						} else {
							a.progress.Success++
							if deleteSource {
								a.progress.Deleted++
							}
						}
					}
					a.progress.Elapsed = time.Since(started).Seconds()
					a.mu.Unlock()
					if err != nil && !errors.Is(err, context.Canceled) {
						a.recordFailure(j.path, err)
					}
					if a.emit != nil {
						for _, index := range j.indices {
							a.emit("file-status-changed", index, status)
						}
					}
					a.publish()
				}
			}()
		}
		a.publish()
	dispatch:
		for _, j := range jobs {
			select {
			case <-ctx.Done():
				break dispatch
			case queue <- j:
			}
		}
		close(queue)
		workers.Wait()

		a.mu.Lock()
		a.progress.Running = false
		a.progress.Cancelled = ctx.Err() != nil
		a.progress.Elapsed = time.Since(started).Seconds()
		a.progress.Current = ""
		a.cancel = nil
		a.mu.Unlock()
		a.publish()
	}()
	return nil
}

// Keep decoding/tag rewrites on the local disk; publish without overwriting.
func convertFile(ctx context.Context, path, savePath string) (warning string, err error) {
	return convertFileWithOptions(ctx, path, savePath, conversionOptions{})
}
func convertFileWithOptions(ctx context.Context, path, savePath string, opts conversionOptions) (warning string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("无效 NCM 数据: %v", r)
		}
	}()
	var original fileProof
	if opts.DeleteSource {
		entry, e := os.Lstat(path)
		if e != nil {
			return "", e
		}
		if !entry.Mode().IsRegular() {
			return "", fmt.Errorf("源文件不是普通文件，不能自动删除")
		}
		original, err = proveFile(ctx, path)
		if err != nil {
			return "", err
		}
	}
	dir := savePath
	if dir == "" {
		dir = filepath.Dir(path)
	}
	stage, e := os.MkdirTemp("", "ncmdump-stage-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(stage)
	n, e := ncmcrypt.NewNeteaseCloudMusic(path)
	if e != nil {
		return "", fmt.Errorf("解析 NCM 失败: %w", e)
	}
	defer n.Close()
	n.Context = ctx
	if _, e = n.Dump(stage); e != nil {
		return "", fmt.Errorf("解密失败: %w", e)
	}
	if _, e = n.FixMetadata(false); e != nil {
		return "", fmt.Errorf("写入元数据失败: %w", e)
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	output := n.GetDumpFilePath()
	target := filepath.Join(dir, filepath.Base(output))
	if e = publishOutput(ctx, output, target); e != nil {
		if errors.Is(e, os.ErrExist) {
			return "", fmt.Errorf("目标文件已存在，未覆盖: %s", target)
		}
		return "", fmt.Errorf("保存失败（不会覆盖已有文件）: %w", e)
	}
	if opts.DeleteSource {
		if e = verifySavedOutput(ctx, output, target); e != nil {
			return "", fmt.Errorf("音频已保存，但读回校验失败；源 NCM 保留: %w", e)
		}
		if e = trashVerifiedSource(ctx, path, original, opts.TrashDir); e != nil {
			return "", fmt.Errorf("音频已保存，但源 NCM 未移至废纸篓: %w", e)
		}
	}
	return n.CoverWarning, nil
}
