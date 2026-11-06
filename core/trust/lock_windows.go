//go:build windows

package trust

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func openStoreLock(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	// A new lock is private to this user plus system/administrators, regardless
	// of the directory's inherited permissions. Do not inherit the handle.
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")(A;;GA;;;SY)(A;;GA;;;BA)")
	if err != nil {
		return nil, err
	}
	attributes := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, &attributes, windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
		_ = file.Close()
		return nil, errors.New("trust lock must be a regular file without links or reparse points")
	}
	security, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	owner, _, err := security.Owner()
	if err != nil || !trustedLockSID(owner, user.User.Sid) {
		_ = file.Close()
		return nil, errors.New("untrusted lock owner")
	}
	dacl, _, err := security.DACL()
	if err != nil || dacl == nil {
		_ = file.Close()
		return nil, errors.New("trust lock must have a private access list")
	}
	const modifying = windows.GENERIC_ALL | windows.GENERIC_WRITE | windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES | windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			_ = file.Close()
			return nil, err
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE {
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if ace.Mask&modifying != 0 && !trustedLockSID(sid, user.User.Sid) {
				_ = file.Close()
				return nil, errors.New("trust lock must not be writable by other users")
			}
		} else if ace.Header.AceType != windows.ACCESS_DENIED_ACE_TYPE {
			_ = file.Close()
			return nil, errors.New("unsupported trust lock access entry")
		}
	}
	return file, nil
}

func trustedLockSID(sid, user *windows.SID) bool {
	return sid != nil && (sid.Equals(user) || sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid))
}

func lockStoreFile(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{})
}

func unlockStoreFile(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
}
