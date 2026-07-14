//go:build windows

package processgroup

import (
	"fmt"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var appJob struct {
	sync.Once
	handle windows.Handle
	err    error
}

func Start(cmd *exec.Cmd) error {
	appJob.Do(func() {
		appJob.handle, appJob.err = windows.CreateJobObject(nil, nil)
		if appJob.err != nil {
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if result, err := windows.SetInformationJobObject(
			appJob.handle,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		); result == 0 {
			appJob.err = err
			// TODO(micro): Explicitly handle or discard CloseHandle's error on the failed initialization path.
			windows.CloseHandle(appJob.handle)
			appJob.handle = 0
		}
	})
	if appJob.err != nil {
		return fmt.Errorf("create child-process job: %w", appJob.err)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(appJob.handle, process)
		// TODO(micro): Do not discard a process-handle close failure; combine it with err or explicitly mark it best effort.
		windows.CloseHandle(process)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("assign child process %d to cleanup job: %w", cmd.Process.Pid, err)
	}
	return nil
}
