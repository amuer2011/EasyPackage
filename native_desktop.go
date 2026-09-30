//go:build darwin || linux

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"easypackage/internal/archive"
)

const (
	dIDQuit                = 11
	dIDPackNav             = 12
	dIDRestoreNav          = 13
	dIDSource              = 100
	dIDSourceBrowse        = 101
	dIDSourceRecent        = 102
	dIDClass               = 103
	dIDLarge               = 104
	dIDLimit               = 105
	dIDUnit                = 106
	dIDOutput              = 107
	dIDOutputBrowse        = 108
	dIDCoverEnabled        = 109
	dIDCover               = 110
	dIDCoverBrowse         = 111
	dIDPasswordEnabled     = 112
	dIDPassword            = 113
	dIDPasswordShow        = 114
	dIDPasswordGenerate    = 115
	dIDPreview             = 116
	dIDStart               = 117
	dIDRestoreFile         = 200
	dIDRestoreBrowse       = 201
	dIDRestoreFolder       = 202
	dIDRestoreFolderBrowse = 203
	dIDRestoreName         = 204
	dIDRestorePassword     = 205
	dIDRestorePasswordShow = 206
	dIDRestoreStart        = 207
	dIDCancel              = 208
	dIDShowOutput          = 209
	dIDRestoreRecent       = 210
	dIDCoverRow            = 211
	dIDPasswordRow         = 212
	dIDRestorePasswordRow  = 213
)

const (
	dialogFolder = iota + 1
	dialogOpenImage
	dialogSaveArchive
	dialogOpenArchive
	dialogSaveJPEG
)

type desktopUI struct {
	app          *app
	mu           sync.Mutex
	busy         bool
	closePending bool
	showPassword bool
	showRestore  bool
	lastJobID    int
	lastOutput   string
	lastIsDir    bool
	quickFolders []folderShortcut
}

var activeDesktop *desktopUI

func runNativeDesktopUI() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	config, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "EasyPackage:", err)
		return
	}
	model := &app{state: job{Status: "idle"}, places: newPlacesStore(filepath.Join(config, "EasyPackage", "preferences.json")), stop: make(chan struct{})}
	ui := &desktopUI{app: model}
	activeDesktop = ui
	platformRun()
	model.mu.Lock()
	if model.cancel != nil {
		model.cancel()
	}
	model.mu.Unlock()
	activeDesktop = nil
}

func desktopReady() {
	ui := activeDesktop
	if ui == nil {
		return
	}
	ui.quickFolders = ui.buildQuickFolders()
	platformSetOptions(dIDUnit, []string{"MB", "GB", "KB"}, 0)
	platformSetOptions(dIDSourceRecent, folderLabels(ui.quickFolders), -1)
	platformSetOptions(dIDRestoreRecent, folderLabels(ui.quickFolders), -1)
	platformSetText(dIDLimit, "100")
	platformSetText(dIDRestoreName, "restored")
	ui.updateOptionalControls()
	ui.setBusy(false)
}

func desktopAction(id int) {
	if ui := activeDesktop; ui != nil {
		ui.command(id)
	}
}

func desktopRefresh() {
	if ui := activeDesktop; ui != nil {
		ui.refresh()
	}
}

func (u *desktopUI) command(id int) {
	u.mu.Lock()
	busy := u.busy
	u.mu.Unlock()
	if busy && id != dIDCancel && id != dIDQuit {
		return
	}
	switch id {
	case dIDPackNav:
		u.hidePasswords()
		platformSetPage(0)
	case dIDRestoreNav:
		u.hidePasswords()
		platformSetPage(1)
	case dIDSourceBrowse:
		if path := platformChoose(dialogFolder, platformGetText(dIDSource)); path != "" {
			platformSetText(dIDSource, path)
			if strings.TrimSpace(platformGetText(dIDOutput)) == "" {
				platformSetText(dIDOutput, filepath.Join(filepath.Dir(path), filepath.Base(path)+".zip"))
			}
			u.remember(path)
		}
	case dIDOutputBrowse:
		kind := dialogSaveArchive
		if platformChecked(dIDCoverEnabled) {
			kind = dialogSaveJPEG
		}
		if path := platformChoose(kind, platformGetText(dIDOutput)); path != "" {
			platformSetText(dIDOutput, path)
			u.remember(filepath.Dir(path))
		}
	case dIDCoverBrowse:
		if path := platformChoose(dialogOpenImage, platformGetText(dIDCover)); path != "" {
			platformSetText(dIDCover, path)
			u.remember(filepath.Dir(path))
		}
	case dIDRestoreBrowse:
		if path := platformChoose(dialogOpenArchive, platformGetText(dIDRestoreFile)); path != "" {
			platformSetText(dIDRestoreFile, path)
			base := filepath.Base(path)
			platformSetText(dIDRestoreName, strings.TrimSuffix(base, filepath.Ext(base))+"-restored")
			u.remember(filepath.Dir(path))
		}
	case dIDRestoreFolderBrowse:
		if path := platformChoose(dialogFolder, platformGetText(dIDRestoreFolder)); path != "" {
			platformSetText(dIDRestoreFolder, path)
			u.remember(path)
		}
	case dIDSourceRecent, dIDRestoreRecent:
		path := platformGetSelected(id)
		if path != "" {
			destination := dIDSource
			if id == dIDRestoreRecent {
				destination = dIDRestoreFolder
			}
			platformSetText(destination, path)
			if id == dIDSourceRecent && strings.TrimSpace(platformGetText(dIDOutput)) == "" {
				platformSetText(dIDOutput, filepath.Join(filepath.Dir(path), filepath.Base(path)+".zip"))
			}
		}
	case dIDCoverEnabled, dIDPasswordEnabled, dIDLarge:
		u.updateOptionalControls()
	case dIDPasswordShow:
		u.showPassword = !u.showPassword
		platformSetSecret(dIDPassword, !u.showPassword)
		platformSetText(dIDPasswordShow, map[bool]string{true: "隐藏", false: "显示"}[u.showPassword])
	case dIDRestorePasswordShow:
		u.showRestore = !u.showRestore
		platformSetSecret(dIDRestorePassword, !u.showRestore)
		platformSetText(dIDRestorePasswordShow, map[bool]string{true: "隐藏", false: "显示"}[u.showRestore])
	case dIDPasswordGenerate:
		u.generatePassword()
	case dIDPreview:
		u.runTask("preview")
	case dIDStart:
		u.runTask("pack")
	case dIDRestoreStart:
		u.runTask("restore")
	case dIDCancel:
		u.cancel()
	case dIDShowOutput:
		if u.lastOutput != "" {
			path := u.lastOutput
			if !u.lastIsDir {
				path = filepath.Dir(path)
			}
			platformOpenPath(path)
		}
	case dIDQuit:
		u.quit()
	}
}

func (u *desktopUI) hidePasswords() {
	if u.showPassword {
		u.showPassword = false
		platformSetSecret(dIDPassword, true)
		platformSetText(dIDPasswordShow, "显示")
	}
	if u.showRestore {
		u.showRestore = false
		platformSetSecret(dIDRestorePassword, true)
		platformSetText(dIDRestorePasswordShow, "显示")
	}
}

func folderLabels(items []folderShortcut) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Label)
	}
	return result
}

func (u *desktopUI) buildQuickFolders() []folderShortcut {
	result := make([]folderShortcut, 0, 12)
	add := func(label, path string) {
		if path == "" {
			return
		}
		for _, existing := range result {
			if sameFolderPath(existing.Path, path) {
				return
			}
		}
		if st, err := os.Stat(path); err == nil && st.IsDir() {
			result = append(result, folderShortcut{Label: label, Path: path})
		}
	}
	for _, folder := range commonFolders() {
		add(folder.Label, folder.Path)
	}
	for _, path := range u.app.places.Recent() {
		add("最近 · "+filepath.Base(path), path)
	}
	return result
}

func (u *desktopUI) remember(path string) {
	_ = u.app.places.Remember(path)
	u.quickFolders = u.buildQuickFolders()
	platformSetOptions(dIDSourceRecent, folderLabels(u.quickFolders), -1)
	platformSetOptions(dIDRestoreRecent, folderLabels(u.quickFolders), -1)
}

func (u *desktopUI) updateOptionalControls() {
	platformSetEnabled(dIDCover, platformChecked(dIDCoverEnabled))
	platformSetEnabled(dIDCoverBrowse, platformChecked(dIDCoverEnabled))
	platformSetEnabled(dIDLimit, platformChecked(dIDLarge))
	platformSetEnabled(dIDUnit, platformChecked(dIDLarge))
	passwordEnabled := platformChecked(dIDPasswordEnabled)
	platformSetEnabled(dIDPassword, passwordEnabled)
	platformSetEnabled(dIDPasswordShow, passwordEnabled)
	platformSetEnabled(dIDPasswordGenerate, passwordEnabled)
	output := strings.TrimSpace(platformGetText(dIDOutput))
	if output != "" {
		desired := map[bool]string{true: ".jpg", false: ".zip"}[platformChecked(dIDCoverEnabled)]
		current := strings.ToLower(filepath.Ext(output))
		if (current == ".zip" || current == ".jpg" || current == ".jpeg") && current != desired {
			platformSetText(dIDOutput, strings.TrimSuffix(output, filepath.Ext(output))+desired)
		}
	}
}

func (u *desktopUI) generatePassword() {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%*-_+"
	b := make([]byte, 32)
	result := make([]byte, 0, 20)
	ceiling := byte(256 - 256%len(alphabet))
	for len(result) < 20 {
		if _, err := rand.Read(b); err != nil {
			platformMessage("随机生成失败", err.Error(), false)
			return
		}
		for _, value := range b {
			if value < ceiling {
				result = append(result, alphabet[int(value)%len(alphabet)])
				if len(result) == 20 {
					break
				}
			}
		}
	}
	u.showPassword = false
	platformSetSecret(dIDPassword, true)
	platformSetText(dIDPassword, string(result))
	platformSetText(dIDPasswordShow, "显示")
	platformSetStatus("已生成 20 位密码 · 默认隐藏", 0, false)
}

func (u *desktopUI) runTask(action string) {
	var opts archive.Options
	if action == "restore" {
		source := strings.TrimSpace(platformGetText(dIDRestoreFile))
		folder := strings.TrimSpace(platformGetText(dIDRestoreFolder))
		name := strings.TrimSpace(platformGetText(dIDRestoreName))
		if source == "" || folder == "" {
			platformMessage("请选择文件和位置", "请选择待还原的 JPG / ZIP 文件和解压位置。", false)
			return
		}
		if !safeDesktopFolderName(name) {
			platformMessage("文件夹名称无效", "请输入有效的新文件夹名称。", false)
			return
		}
		opts = archive.Options{Source: source, Output: filepath.Join(folder, name), Password: platformGetText(dIDRestorePassword)}
	} else {
		source := strings.TrimSpace(platformGetText(dIDSource))
		output := strings.TrimSpace(platformGetText(dIDOutput))
		if source == "" || (output == "" && action != "preview") {
			platformMessage("请选择项目和保存位置", "请选择要打包的项目文件夹和输出压缩包。", false)
			return
		}
		if output == "" {
			output = filepath.Join(filepath.Dir(source), "."+filepath.Base(source)+"-preview.zip")
		}
		cover := ""
		if action != "preview" && platformChecked(dIDCoverEnabled) {
			cover = strings.TrimSpace(platformGetText(dIDCover))
			if cover == "" {
				platformMessage("请选择 JPG 封面", "启用了 JPG 封面，请选择一张有效的 JPG 图片。", false)
				return
			}
		}
		maxBytes := int64(0)
		if platformChecked(dIDLarge) {
			value, err := strconv.ParseFloat(strings.TrimSpace(platformGetText(dIDLimit)), 64)
			if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				platformMessage("大小无效", "请输入有效的大文件大小阈值。", false)
				return
			}
			unit := platformGetSelected(dIDUnit)
			scale := float64(1024 * 1024)
			if unit == "GB" {
				scale *= 1024
			} else if unit == "KB" {
				scale = 1024
			}
			bytes := value * scale
			if bytes < 1 || bytes > float64(math.MaxInt64) {
				platformMessage("大小超出范围", "阈值超出支持范围。", false)
				return
			}
			maxBytes = int64(math.Round(bytes))
		}
		password := ""
		if action != "preview" && platformChecked(dIDPasswordEnabled) {
			password = platformGetText(dIDPassword)
			if password == "" {
				platformMessage("请输入密码", "启用密码保护后，请输入压缩密码。", false)
				return
			}
		}
		opts = archive.Options{Source: source, Output: output, FilterClass: platformChecked(dIDClass), MaxBytes: maxBytes, Cover: cover, Password: password}
	}
	if action == "restore" {
		if _, err := os.Lstat(opts.Output); err == nil {
			platformMessage("目标文件夹已存在", "请更换新文件夹名称；不会覆盖或合并已有文件夹。", false)
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			platformMessage("还原位置无效", err.Error(), false)
			return
		}
	} else if action == "pack" {
		if info, err := os.Lstat(opts.Output); err == nil {
			if !info.Mode().IsRegular() {
				platformMessage("输出位置无效", "输出位置不是普通文件。", false)
				return
			}
			if !platformMessage("确认覆盖", "目标文件已存在：\n"+opts.Output+"\n\n要替换这个文件吗？", true) {
				return
			}
			opts.Overwrite = true
		} else if !errors.Is(err, os.ErrNotExist) {
			platformMessage("输出位置无效", err.Error(), false)
			return
		}
	}
	u.app.mu.Lock()
	if u.app.state.Status == "running" {
		u.app.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.app.cancel = cancel
	u.app.state = job{ID: u.app.state.ID + 1, Status: "running", Phase: "正在扫描项目", Preview: action == "preview"}
	id := u.app.state.ID
	u.app.mu.Unlock()
	u.mu.Lock()
	u.busy = true
	u.lastJobID = id
	u.mu.Unlock()
	platformSetBusy(true)
	platformSetStatus(map[string]string{"restore": "正在验证并直接解压…", "pack": "正在准备打包…", "preview": "正在扫描项目文件…"}[action], 0, true)
	go u.app.run(ctx, task{Options: opts, Action: action})
}

func (u *desktopUI) cancel() {
	u.app.mu.Lock()
	if u.app.cancel != nil {
		u.app.cancel()
	}
	u.app.mu.Unlock()
	platformSetStatus("正在安全取消任务…", 0, true)
}

func (u *desktopUI) quit() {
	u.mu.Lock()
	busy := u.busy
	u.mu.Unlock()
	if busy {
		if !platformMessage("正在处理", "当前任务尚未完成。退出会取消任务，是否继续？", true) {
			return
		}
		u.mu.Lock()
		u.closePending = true
		u.mu.Unlock()
		u.cancel()
		return
	}
	platformQuit()
}

func (u *desktopUI) refresh() {
	u.app.mu.Lock()
	state := u.app.state
	u.app.mu.Unlock()
	u.mu.Lock()
	busy := u.busy
	u.mu.Unlock()
	if !busy {
		return
	}
	status := state.Phase
	if state.Status == "running" && state.Current != "" {
		status = state.Current
	}
	percent := 0
	marquee := true
	if state.Status == "running" && state.Plan != nil && state.Plan.Bytes > 0 {
		percent = int(math.Min(100, float64(state.Done)*100/float64(state.Plan.Bytes)))
		marquee = false
		status = fmt.Sprintf("%s   ·   %d%%", state.Phase, percent)
	}
	platformSetStatus(status, percent, marquee)
	if state.Status == "running" {
		return
	}
	u.mu.Lock()
	u.busy = false
	closePending := u.closePending
	u.mu.Unlock()
	platformSetBusy(false)
	if state.Status == "error" {
		platformMessage("任务未完成", state.Error, false)
	} else if state.Status == "cancelled" {
		platformMessage("任务已取消", "临时文件已清理；已有输出文件保持不变。", false)
	} else if state.Status == "done" {
		if state.Output != "" {
			u.lastOutput = state.Output
			u.lastIsDir = state.OutputDirectory
			platformSetEnabled(dIDShowOutput, true)
		}
		message := "输出文件：\n" + state.Output
		if state.Preview {
			message = "文件清单预览"
			if state.Plan != nil {
				message += fmt.Sprintf("\n\n%d 个文件 · %s\n过滤 %d 个文件 / %d 个目录", state.Plan.Files, humanBytes(state.Plan.Bytes), state.Plan.Filtered, state.Plan.ExcludedDirs)
				for index, item := range state.Plan.Items {
					if index == 45 {
						message += "\n…（更多内容请查看完整清单）"
						break
					}
					if item.Directory {
						message += "\n[目录] " + item.Name
					} else {
						message += "\n" + item.Name
					}
				}
			}
		} else if state.OutputDirectory {
			message = "项目已直接解压到：\n" + state.Output
		}
		if state.Plan != nil && !state.Preview {
			message += fmt.Sprintf("\n\n%d 个文件 · %s · 过滤 %d 个文件 / %d 个目录", state.Plan.Files, humanBytes(state.Plan.Bytes), state.Plan.Filtered, state.Plan.ExcludedDirs)
		}
		platformMessage("EasyPackage · 完成", message, false)
	}
	if closePending {
		platformQuit()
	}
}

func (u *desktopUI) setBusy(busy bool) {
	for _, id := range []int{dIDSource, dIDSourceBrowse, dIDSourceRecent, dIDClass, dIDLarge, dIDLimit, dIDUnit, dIDOutput, dIDOutputBrowse, dIDCoverEnabled, dIDCover, dIDCoverBrowse, dIDPasswordEnabled, dIDPassword, dIDPasswordShow, dIDPasswordGenerate, dIDPreview, dIDStart, dIDRestoreFile, dIDRestoreBrowse, dIDRestoreFolder, dIDRestoreFolderBrowse, dIDRestoreRecent, dIDRestoreName, dIDRestorePassword, dIDRestorePasswordShow, dIDRestoreStart, dIDPackNav, dIDRestoreNav} {
		platformSetEnabled(id, !busy)
	}
	platformSetEnabled(dIDCancel, busy)
	platformSetEnabled(dIDShowOutput, !busy && u.lastOutput != "")
	if !busy {
		u.updateOptionalControls()
	}
}

func safeDesktopFolderName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, "/\\:\x00<>\"|?*") && !strings.HasSuffix(name, ".") && !strings.HasSuffix(name, " ")
}

func humanBytes(n int64) string {
	if n >= 1073741824 {
		return fmt.Sprintf("%.2f GB", float64(n)/1073741824)
	}
	if n >= 1048576 {
		return fmt.Sprintf("%.2f MB", float64(n)/1048576)
	}
	if n >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%d B", n)
}
