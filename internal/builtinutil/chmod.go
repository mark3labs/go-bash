package builtinutil

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseChmodMode parses a chmod mode string (numeric like "755" or
// symbolic like "u+x,g-w,o=r") and applies it on top of the given
// current file mode, returning the resulting mode.
//
// Numeric mode: parsed as octal; the returned value masks current
// special bits (setuid/setgid/sticky) when only three digits supplied
// — coreutils behavior.
//
// Symbolic mode: one or more comma-separated clauses. Each clause is
// "[ugoa]*[-+=][rwxXst]*". Empty user-set defaults to "a", subject
// to umask (we do NOT apply umask here — the caller decides). 'X' is
// a conditional execute (only set when target is a directory or any
// execute bit is already set — caller passes isDir).
func ParseChmodMode(spec string, current os.FileMode, isDir bool) (os.FileMode, error) {
	if spec == "" {
		return current, fmt.Errorf("empty mode")
	}
	// Numeric form: optional leading 0, all digits 0-7.
	if isOctalMode(spec) {
		v, err := strconv.ParseUint(spec, 8, 32)
		if err != nil {
			return current, err
		}
		return os.FileMode(v) & 0o7777, nil
	}
	mode := current & 0o7777
	clauses := strings.Split(spec, ",")
	for _, clause := range clauses {
		if clause == "" {
			return current, fmt.Errorf("invalid mode: %q", spec)
		}
		m, err := applySymbolicClause(clause, mode, isDir)
		if err != nil {
			return current, err
		}
		mode = m
	}
	return mode, nil
}

func isOctalMode(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '7' {
			return false
		}
	}
	return true
}

func applySymbolicClause(clause string, mode os.FileMode, isDir bool) (os.FileMode, error) {
	who := uint32(0)
	whoMask := uint32(0)
	i := 0
loop:
	for i < len(clause) {
		switch clause[i] {
		case 'u':
			who |= 0o700
			whoMask |= 0o4700
		case 'g':
			who |= 0o070
			whoMask |= 0o2070
		case 'o':
			who |= 0o007
			whoMask |= 0o1007
		case 'a':
			who |= 0o777
			whoMask |= 0o7777
		default:
			break loop
		}
		i++
	}
	if who == 0 {
		who = 0o777
		whoMask = 0o7777
	}
	if i >= len(clause) {
		return mode, fmt.Errorf("missing operator in mode clause %q", clause)
	}
	op := clause[i]
	if op != '+' && op != '-' && op != '=' {
		return mode, fmt.Errorf("invalid operator %q in mode clause %q", op, clause)
	}
	i++
	perms := uint32(0)
	for ; i < len(clause); i++ {
		switch clause[i] {
		case 'r':
			perms |= 0o444
		case 'w':
			perms |= 0o222
		case 'x':
			perms |= 0o111
		case 'X':
			if isDir || (uint32(mode)&0o111) != 0 {
				perms |= 0o111
			}
		case 's':
			perms |= 0o6000 // setuid + setgid
		case 't':
			perms |= 0o1000 // sticky
		default:
			return mode, fmt.Errorf("invalid permission %q in mode clause %q", clause[i], clause)
		}
	}
	// Restrict perms to selected who-mask.
	perms &= whoMask
	switch op {
	case '+':
		mode |= os.FileMode(perms)
	case '-':
		mode &^= os.FileMode(perms)
	case '=':
		// Clear the who-bits and set the perms bits.
		mode = (mode &^ os.FileMode(who)) | os.FileMode(perms)
	}
	return mode & 0o7777, nil
}
