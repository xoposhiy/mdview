package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"mdview/internal/applog"
	"mdview/internal/viewer"
)

// The release script sets buildID to a 19-digit, increasing release number.
// Local builds without a release number run as standalone viewers.
var buildID string

const handoffTimeout = 15 * time.Second

func watchForRelease(ctx context.Context, v *viewer.Viewer, controlPort int) {
	exe, err := os.Executable()
	if err != nil {
		applog.Write("Cannot find the installed executable: %v", err)
		return
	}
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	last := buildID
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		id := publishedRelease(exe + ".version")
		if id <= last {
			continue
		}
		err := restartWindow(ctx, v, exe, id, controlPort)
		if err == nil {
			v.Quit()
			return
		}
		if errors.Is(err, viewer.ErrWindowBusy) {
			continue
		}
		last = id
		applog.Write("Cannot update window to release %s: %v", id, err)
	}
}

func publishedRelease(path string) string {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	// Let the publisher replace the small version file while it is open.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return ""
	}
	file := os.NewFile(uintptr(handle), path)
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 32))
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(data))
	if _, err := strconv.ParseUint(id, 10, 64); len(id) != 19 || err != nil {
		return ""
	}
	return id
}

func restartWindow(ctx context.Context, v *viewer.Viewer, exe, id string, controlPort int) error {
	state, err := v.CaptureWindow()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, handoffTimeout)
	defer cancel()
	args := []string{"--resume=" + id}
	if controlPort >= 0 {
		// The old window still owns its port until handoff completes.
		args = append(args, "-control-port=0")
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	defer out.Close()
	if err := cmd.Start(); err != nil {
		return err
	}
	shown := false
	defer func() {
		if shown {
			go cmd.Wait()
			return
		}
		cmd.Process.Kill()
		cmd.Wait()
	}()
	enc := json.NewEncoder(in)
	if err := enc.Encode(state); err != nil {
		return err
	}
	replies := bufio.NewReader(out)
	if err := expectReply(ctx, replies, "ready\n"); err != nil {
		return err
	}
	foreground, err := v.PrepareHandoff(state.File, cmd.Process.Pid)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := enc.Encode(foreground); err != nil {
		return err
	}
	if err := expectReply(ctx, replies, "shown\n"); err != nil {
		return err
	}
	shown = true
	return nil
}

func expectReply(ctx context.Context, reader *bufio.Reader, want string) error {
	result := make(chan error, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if err == nil && line != want {
			err = fmt.Errorf("unexpected reply %q", line)
		}
		result <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-result:
		if err != nil {
			return fmt.Errorf("waiting for %q: %w", strings.TrimSpace(want), err)
		}
		return nil
	}
}

type resumeInput struct {
	state viewer.WindowState
	dec   *json.Decoder
}

func readResume(id string) (*resumeInput, error) {
	if id != buildID {
		return nil, errors.New("replacement is a different release")
	}
	input := &resumeInput{dec: json.NewDecoder(os.Stdin)}
	if err := input.dec.Decode(&input.state); err != nil {
		return nil, err
	}
	if input.state.File == "" {
		return nil, errors.New("replacement has no file to open")
	}
	return input, nil
}

func (r *resumeInput) show(ctx context.Context, v *viewer.Viewer) error {
	readyCtx, cancel := context.WithTimeout(ctx, handoffTimeout)
	defer cancel()
	if err := v.WaitReady(readyCtx); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(os.Stdout, "ready"); err != nil {
		return err
	}
	var foreground bool
	if err := r.dec.Decode(&foreground); errors.Is(err, io.EOF) {
		// The release smoke check closes stdin once it has sent the state.
		return v.Quit()
	} else if err != nil {
		return err
	}
	if err := v.ShowReplacement(r.state, foreground); err != nil {
		return err
	}
	_, err := fmt.Fprintln(os.Stdout, "shown")
	return err
}
