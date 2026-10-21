package privatefs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func createTempDirectory(parent, prefix string) (string, error) {
	if parent == "" {
		parent = os.TempDir()
	}
	if strings.ContainsAny(prefix, `/\`) {
		return "", errors.New("temporary directory prefix must not contain path separators")
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", fmt.Errorf("create temporary directory parent: %w", err)
	}
	for range 100 {
		randomSuffix := make([]byte, 12)
		if _, err := rand.Read(randomSuffix); err != nil {
			return "", fmt.Errorf("generate temporary directory name: %w", err)
		}
		path := filepath.Join(parent, prefix+hex.EncodeToString(randomSuffix))
		if err := createPrivateDirectory(path); err != nil {
			if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
				continue
			}
			return "", fmt.Errorf("create private temporary directory: %w", err)
		}
		if err := ensurePrivateDirectory(path); err != nil {
			return "", errors.Join(err, os.Remove(path))
		}
		return path, nil
	}
	return "", errors.New("cannot allocate unique private temporary directory")
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create private directory parent: %w", err)
	}

	err := createPrivateDirectory(path)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return fmt.Errorf("create private directory: %w", err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect private directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("directory must be private")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("identify private directory owner: %w", err)
	}
	if err := validatePrivateDirectoryACL(path, user.User.Sid); err != nil {
		return fmt.Errorf("directory must be private: %w", err)
	}
	return nil
}

func createPrivateDirectory(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("identify private directory owner: %w", err)
	}
	userSID := user.User.Sid
	descriptor, err := windows.SecurityDescriptorFromString(fmt.Sprintf(
		"O:%sD:P(A;OICI;GA;;;%s)(A;OICI;GA;;;SY)(A;OICI;GA;;;BA)",
		userSID.String(),
		userSID.String(),
	))
	if err != nil {
		return fmt.Errorf("create private directory ACL: %w", err)
	}
	attributes := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	widePath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("encode private directory path: %w", err)
	}
	if err := windows.CreateDirectory(widePath, &attributes); err != nil {
		return err
	}
	return nil
}

func validatePrivateDirectoryACL(path string, userSID *windows.SID) error {
	securityDescriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("read private directory ACL: %w", err)
	}
	owner, _, err := securityDescriptor.Owner()
	if err != nil {
		return fmt.Errorf("read private directory owner: %w", err)
	}
	if !owner.Equals(userSID) {
		return errors.New("directory must be owned by the current user")
	}
	control, _, err := securityDescriptor.Control()
	if err != nil {
		return fmt.Errorf("read private directory ACL control: %w", err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("directory ACL must not inherit permissions")
	}
	dacl, _, err := securityDescriptor.DACL()
	if err != nil {
		return fmt.Errorf("read private directory DACL: %w", err)
	}
	if dacl.AceCount != 3 {
		return errors.New("directory ACL grants access to unexpected principals")
	}

	systemSID, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		return fmt.Errorf("resolve Windows SYSTEM SID: %w", err)
	}
	adminsSID, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		return fmt.Errorf("resolve Windows Administrators SID: %w", err)
	}
	expectedPrincipals := map[string]bool{
		userSID.String():   false,
		systemSID.String(): false,
		adminsSID.String(): false,
	}
	const fullFileAccessMask = windows.ACCESS_MASK(0x001F01FF)
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return fmt.Errorf("read private directory ACL entry: %w", err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
			ace.Header.AceFlags != windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE ||
			ace.Mask != fullFileAccessMask {
			return errors.New("directory ACL grants unexpected permissions")
		}
		principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		allowed, exists := expectedPrincipals[principal]
		if !exists || allowed {
			return errors.New("directory ACL grants access to unexpected principals")
		}
		expectedPrincipals[principal] = true
	}
	for _, present := range expectedPrincipals {
		if !present {
			return errors.New("directory ACL is missing a required principal")
		}
	}
	return nil
}
