//go:build unix

package store

import "syscall"

func tightenProcessUmask() {
	syscall.Umask(0o077)
}
