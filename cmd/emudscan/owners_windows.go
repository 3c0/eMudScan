package main

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	modadvapi32               = sysDLL("advapi32.dll")
	procGetNamedSecurityInfoW = modadvapi32.NewProc("GetNamedSecurityInfoW")
	procLocalFree             = modkernel.NewProc("LocalFree")
)

// readFileSecurity reads the owner of a file or folder, and the accounts that its
// permissions (its DACL) allow to change it.
func readFileSecurity(path string) (fileSec, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fileSec{}, err
	}
	const seFileObject, ownerInformation, daclInformation = 1, 0x1, 0x4
	var owner *syscall.SID
	var dacl, sd unsafe.Pointer
	r, _, _ := procGetNamedSecurityInfoW.Call(uintptr(unsafe.Pointer(p)), seFileObject, ownerInformation|daclInformation,
		uintptr(unsafe.Pointer(&owner)), 0, uintptr(unsafe.Pointer(&dacl)), 0, uintptr(unsafe.Pointer(&sd)))
	if r != 0 {
		return fileSec{}, syscall.Errno(r)
	}
	defer procLocalFree.Call(uintptr(sd))
	if owner == nil {
		return fileSec{}, errors.New("no owner")
	}
	var s fileSec
	if s.owner, err = owner.String(); err != nil {
		return fileSec{}, err
	}
	if dacl == nil { // no permissions at all: anyone may change it
		s.writers = []string{sidEveryone}
		return s, nil
	}
	// ACL: revision, padding, size (uint16), number of entries (uint16), padding;
	// then each entry (ACE): type, flags, size (uint16), and for an "allow" entry the
	// rights (uint32) and the SID
	size := uintptr(*(*uint16)(unsafe.Add(dacl, 2)))
	count := int(*(*uint16)(unsafe.Add(dacl, 4)))
	for i, off := 0, uintptr(8); i < count && off+8 <= size; i++ {
		ace := unsafe.Add(dacl, off)
		typ, aceSize := *(*byte)(ace), uintptr(*(*uint16)(unsafe.Add(ace, 2)))
		if aceSize < 8 || off+aceSize > size {
			break
		}
		// ACCESS_ALLOWED_ACE_TYPE, ACCESS_ALLOWED_CALLBACK_ACE_TYPE
		if (typ == 0 || typ == 9) && aceSize >= 16 && *(*uint32)(unsafe.Add(ace, 4))&changeRights != 0 {
			if sid, err := (*syscall.SID)(unsafe.Add(ace, 8)).String(); err == nil {
				s.writers = append(s.writers, sid)
			}
		}
		off += aceSize
	}
	return s, nil
}

// readAccountName names an account (DOMAIN\name), or gives its SID.
func readAccountName(sid string) string {
	s, err := syscall.StringToSid(sid)
	if err != nil {
		return sid
	}
	account, domain, _, err := s.LookupAccount("")
	switch {
	case err != nil || account == "":
		return sid
	case domain == "":
		return account
	}
	return domain + `\` + account
}

// readCurrentUserSID is the SID of the user running eMudScan.
func readCurrentUserSID() string {
	t, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return ""
	}
	defer t.Close()
	u, err := t.GetTokenUser()
	if err != nil {
		return ""
	}
	s, err := u.User.Sid.String()
	if err != nil {
		return ""
	}
	return s
}
