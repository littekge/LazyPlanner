package main

import "testing"

// TestConnFlagsClientRequiresAllCredentials pins that a CalDAV client is built
// only when the URL, username, AND password are all present — a partial set (any
// one missing) must return an error, never a client with empty credentials.
// Guards the pass-16 canary: flipping the credential guard's || to && let a
// url+username-without-password combination build a client anyway.
func TestConnFlagsClientRequiresAllCredentials(t *testing.T) {
	str := func(s string) *string { return &s }

	cases := []struct {
		name                  string
		url, username, passwd string
		wantErr               bool
	}{
		{"all present", "http://localhost/dav", "me", "pw", false},
		{"missing password", "http://localhost/dav", "me", "", true},
		{"missing username", "http://localhost/dav", "", "pw", true},
		{"missing url", "", "me", "pw", true},
		{"all empty", "", "", "", true},
	}
	for _, tc := range cases {
		cf := connFlags{url: str(tc.url), username: str(tc.username), password: str(tc.passwd)}
		client, err := cf.client()
		switch {
		case tc.wantErr && err == nil:
			t.Errorf("%s: client() returned no error (and client=%v), want an error", tc.name, client != nil)
		case !tc.wantErr && err != nil:
			t.Errorf("%s: client() error = %v, want a client", tc.name, err)
		case !tc.wantErr && client == nil:
			t.Errorf("%s: client() returned nil client with no error", tc.name)
		}
	}
}
