package main

import (
	"context"
	"devcourse-finder/auth/password"
	"fmt"
)

// Administrative commands run before database configuration and never print
// keyring contents, raw parsing errors, paths or credential material.
func passwordKeyringCommand(args []string) error {
	invalid := func() error {
		return commandFailure("password_keyring", "invalid_arguments", "Usage: password-keyring init PATH ID | rotate SOURCE NEW_PATH ID | check PATH | probe KEYRING NEW_PROBE | verify KEYRING PROBE", nil)
	}
	if len(args) == 0 {
		return invalid()
	}
	var err error
	switch {
	case args[0] == "init" && len(args) == 3:
		err = password.GenerateKeyring(args[1], args[2])
	case args[0] == "rotate" && len(args) == 4:
		err = password.RotateKeyring(args[1], args[2], args[3])
	case args[0] == "check" && len(args) == 2:
		_, err = password.LoadKeyring(args[1])
	case args[0] == "probe" && len(args) == 3:
		err = password.WriteRecoveryProbe(context.Background(), args[1], args[2])
	case args[0] == "verify" && len(args) == 3:
		err = password.VerifyRecoveryProbe(context.Background(), args[1], args[2])
	default:
		return invalid()
	}
	if err != nil {
		return commandFailure("password_keyring", "password_keyring_failed", "Password keyring operation failed; values hidden", nil)
	}
	fmt.Println("Password keyring operation succeeded; values hidden")
	return nil
}
