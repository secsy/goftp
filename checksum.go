// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"fmt"
	"strings"
)

// Checksum algorithms in descending order of preference, for the case
// where the caller has not named one.
//
// Ordered by collision resistance, not by speed. A caller who wanted
// speed would say so; a caller who did not is comparing files, and a
// CRC32 that matches is much weaker evidence than a SHA-256 that does.
var checksumPreference = []string{
	"SHA-512",
	"SHA-256",
	"SHA-1",
	"MD5",
	"CRC32",
}

// The legacy commands, which predate HASH and are what most servers
// that support checksums at all actually have. The name each is known
// by in FEAT maps to the algorithm it computes.
var legacyChecksumCommands = []struct {
	command   string
	algorithm string
}{
	{"XSHA512", "SHA-512"},
	{"XSHA256", "SHA-256"},
	{"XSHA1", "SHA-1"},
	{"XSHA", "SHA-1"},
	{"XMD5", "MD5"},
	{"MD5", "MD5"},
	{"XCRC", "CRC32"},
}

// ChecksumAlgorithms returns the checksum algorithms the server can
// compute, strongest first, or an empty slice if it can compute none.
//
// Names are as they appear in the HASH extension — "SHA-256", "MD5",
// "CRC32" — regardless of whether the server offers them through HASH or
// through one of the older per-algorithm commands, so a caller does not
// have to know which the server implements.
func (c *Client) ChecksumAlgorithms() ([]string, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return nil, err
	}
	defer c.returnConn(pconn)

	return pconn.checksumAlgorithms(), nil
}

// Checksum asks the server for a checksum of file "path", computed on
// the server, and returns it as lowercase hexadecimal.
//
// An empty algorithm asks for the strongest the server offers, which is
// what a caller comparing a remote file against a local one wants and
// saves them a round trip discovering what is available.
//
// The result is always lowercase hex, whichever command answered.
// Servers are inconsistent — proftpd's HASH replies in lowercase and its
// XSHA256 in uppercase — and a caller comparing against
// encoding/hex.EncodeToString would otherwise be right only half the
// time, for reasons nothing in their code would explain.
//
// # Errors
//
// An error if the server computes no checksums at all, if it cannot
// compute the algorithm asked for, or if "path" is not a file it can
// read. Note that a checksum is not free: the server reads the whole
// file to answer, so this is cheaper than downloading it but not cheap.
func (c *Client) Checksum(path string, algorithm string) (string, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return "", err
	}
	defer c.returnConn(pconn)

	available := pconn.checksumAlgorithms()
	if len(available) == 0 {
		return "", ftpError{err: fmt.Errorf(
			"the server computes no checksums: it advertises neither HASH nor any of the " +
				"older per-algorithm commands")}
	}

	if algorithm == "" {
		algorithm = available[0]
	} else if !containsFold(available, algorithm) {
		return "", ftpError{err: fmt.Errorf(
			"the server cannot compute %s; it offers %s",
			algorithm, strings.Join(available, ", "))}
	}

	// HASH is the standards-track command and reports which algorithm it
	// used, so it is preferred where the server has it.
	if _, ok := pconn.features["HASH"]; ok {
		return pconn.hashChecksum(path, algorithm)
	}

	for _, legacy := range legacyChecksumCommands {
		if !strings.EqualFold(legacy.algorithm, algorithm) {
			continue
		}
		if _, ok := pconn.features[legacy.command]; !ok {
			continue
		}
		return pconn.legacyChecksum(legacy.command, path)
	}

	return "", ftpError{err: fmt.Errorf("no command available for %s", algorithm)}
}

// checksumAlgorithms reads what this connection's server offers, from
// the features it announced at login.
func (pconn *persistentConn) checksumAlgorithms() []string {
	found := make(map[string]bool)

	// HASH lists its algorithms as its feature argument, marking the
	// current default with a trailing asterisk:
	//
	//	HASH:CRC32;MD5;SHA-1*;SHA-256;SHA-512;
	if arg, ok := pconn.features["HASH"]; ok {
		for _, algo := range strings.Split(arg, ";") {
			algo = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(algo), "*"))
			if algo != "" {
				found[strings.ToUpper(algo)] = true
			}
		}
	}

	// The older commands are announced by name, one per algorithm.
	for _, legacy := range legacyChecksumCommands {
		if _, ok := pconn.features[legacy.command]; ok {
			found[strings.ToUpper(legacy.algorithm)] = true
		}
	}

	// Returned in preference order rather than the server's, so the
	// first entry is the one Checksum would choose.
	var out []string
	for _, algo := range checksumPreference {
		if found[strings.ToUpper(algo)] {
			out = append(out, algo)
			delete(found, strings.ToUpper(algo))
		}
	}
	// Anything the server offers that this package has no opinion about
	// still gets reported, so an unusual algorithm is usable even though
	// it will never be chosen automatically.
	for algo := range found {
		out = append(out, algo)
	}

	return out
}

// hashChecksum computes a checksum with the HASH command.
func (pconn *persistentConn) hashChecksum(path, algorithm string) (string, error) {
	// HASH computes whatever OPTS HASH last selected, so the algorithm
	// has to be chosen before asking. It is per-connection state, which
	// is why this runs on the connection the HASH will run on.
	if _, _, err := pconn.sendCommand("OPTS HASH %s", algorithm); err != nil {
		return "", err
	}

	code, msg, err := pconn.sendCommand("HASH %s", path)
	if err != nil {
		return "", err
	}
	if !positiveCompletionReply(code) {
		return "", ftpError{code: code, msg: msg}
	}

	// The reply's last line is:
	//
	//	SHA-256 0-12 7e838193…4b8a lorem.txt
	//
	// preceded on some servers by a progress line ("Computing SHA-256
	// digest"), which is why the last line is the one read rather than
	// the first.
	fields := strings.Fields(lastLine(msg))
	if len(fields) < 3 {
		return "", ftpError{err: fmt.Errorf("cannot read a checksum from %q", msg)}
	}

	return strings.ToLower(fields[2]), nil
}

// legacyChecksum computes a checksum with one of the pre-HASH commands.
func (pconn *persistentConn) legacyChecksum(command, path string) (string, error) {
	code, msg, err := pconn.sendCommand("%s %s", command, path)
	if err != nil {
		return "", err
	}
	if !positiveCompletionReply(code) {
		return "", ftpError{code: code, msg: msg}
	}

	// These reply with the digest and nothing else, again possibly after
	// a progress line. Some servers prefix the path; the digest is the
	// last field either way.
	fields := strings.Fields(lastLine(msg))
	if len(fields) == 0 {
		return "", ftpError{err: fmt.Errorf("cannot read a checksum from %q", msg)}
	}

	return strings.ToLower(fields[len(fields)-1]), nil
}

// lastLine returns the final non-empty line of a possibly multi-line
// reply.
func lastLine(msg string) string {
	lines := strings.Split(strings.ReplaceAll(msg, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return strings.TrimSpace(lines[i])
		}
	}
	return ""
}

func containsFold(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}
