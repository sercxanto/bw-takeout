package bwcli

import (
	"os"
	"runtime"
	"strings"
)

// inherited are the variables copied from the calling environment when set
// (design D3). PATH is needed because the npm layout starts bw through
// "env node". The Windows variables are needed to start a process and a
// command script.
var inherited = []string{
	"PATH", "HOME", "USERPROFILE", "SystemRoot", "windir", "ComSpec", "PATHEXT",
	"TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL", "LC_CTYPE", "TERM",
}

// environ builds the environment of one bw process from nothing: the
// allowlisted variables of the caller and the values the client sets. The
// session key is added only when withSession is set and the client holds
// one. BW_NOINTERACTION is set unless interactive is set.
func (c *Client) environ(interactive, withSession bool) []string {
	env := allowed(os.Environ(), runtime.GOOS == "windows")
	env = append(env, "BITWARDENCLI_APPDATA_DIR="+c.cfg.DataDir)
	if !interactive {
		env = append(env, "BW_NOINTERACTION=true")
	}
	if withSession && c.session != nil {
		env = append(env, "BW_SESSION="+string(c.session))
	}
	if c.cfg.CAFile != "" {
		env = append(env, "NODE_EXTRA_CA_CERTS="+c.cfg.CAFile)
	}
	return env
}

// allowed returns the entries of environ whose names are on the allowlist.
// With fold set, names are compared without regard to case, as Windows
// does.
func allowed(environ []string, fold bool) []string {
	var out []string
	for _, e := range environ {
		// On Windows an entry can start with "=", such as "=C:=C:\x", so
		// the search for the separator starts at the second byte.
		if e == "" {
			continue
		}
		i := strings.IndexByte(e[1:], '=')
		if i < 0 {
			continue
		}
		name := e[:i+1]
		for _, a := range inherited {
			if name == a || (fold && strings.EqualFold(name, a)) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}
