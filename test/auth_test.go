// Copyright 2012-2013 Apcera Inc. All rights reserved.

package test

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/nats-io/gnatsd/auth"
	"github.com/nats-io/gnatsd/server"
)

func doAuthConnect(t tLogger, c net.Conn, token, user, pass string) {
	cs := fmt.Sprintf("CONNECT {\"verbose\":true,\"auth_token\":\"%s\",\"user\":\"%s\",\"pass\":\"%s\"}\r\n", token, user, pass)
	sendProto(t, c, cs)
}

func testInfoForAuth(t tLogger, infojs []byte) bool {
	var sinfo server.Info
	err := json.Unmarshal(infojs, &sinfo)
	if err != nil {
		t.Fatalf("Could not unmarshal INFO json: %v\n", err)
	}
	return sinfo.AuthRequired
}

func expectAuthRequired(t tLogger, c net.Conn) {
	buf := expectResult(t, c, infoRe)
	infojs := infoRe.FindAllSubmatch(buf, 1)[0][1]
	if testInfoForAuth(t, infojs) != true {
		t.Fatalf("Expected server to require authorization: '%s'", infojs)
	}
}

////////////////////////////////////////////////////////////
// The authorization token version
////////////////////////////////////////////////////////////

const AUTH_PORT = 10422
const AUTH_TOKEN = "_YZZ22_"

func runAuthServerWithToken() *server.Server {
	opts := DefaultTestOptions
	opts.Port = AUTH_PORT
	opts.Authorization = AUTH_TOKEN
	return RunServerWithAuth(&opts, &auth.Token{Token: AUTH_TOKEN})
}

func TestNoAuthClient(t *testing.T) {
	s := runAuthServerWithToken()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, "", "", "")
	expectResult(t, c, errRe)
}

func TestAuthClientBadToken(t *testing.T) {
	s := runAuthServerWithToken()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, "ZZZ", "", "")
	expectResult(t, c, errRe)
}

func TestAuthClientNoConnect(t *testing.T) {
	s := runAuthServerWithToken()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	// This is timing dependent..
	time.Sleep(server.AUTH_TIMEOUT)
	expectResult(t, c, errRe)
}

func TestAuthClientGoodConnect(t *testing.T) {
	s := runAuthServerWithToken()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, AUTH_TOKEN, "", "")
	expectResult(t, c, okRe)
}

func TestAuthClientFailOnEverythingElse(t *testing.T) {
	s := runAuthServerWithToken()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	sendProto(t, c, "PUB foo 2\r\nok\r\n")
	expectResult(t, c, errRe)
}

////////////////////////////////////////////////////////////
// The username/password version
////////////////////////////////////////////////////////////

const AUTH_USER = "derek"
const AUTH_PASS = "foobar"

func runAuthServerWithUserPass() *server.Server {
	opts := DefaultTestOptions
	opts.Port = AUTH_PORT
	opts.Username = AUTH_USER
	opts.Password = AUTH_PASS

	auth := &auth.Plain{Username: AUTH_USER, Password: AUTH_PASS}
	return RunServerWithAuth(&opts, auth)
}

func TestNoUserOrPasswordClient(t *testing.T) {
	s := runAuthServerWithUserPass()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, "", "", "")
	expectResult(t, c, errRe)
}

func TestBadUserClient(t *testing.T) {
	s := runAuthServerWithUserPass()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, "", "derekzz", AUTH_PASS)
	expectResult(t, c, errRe)
}

func TestBadPasswordClient(t *testing.T) {
	s := runAuthServerWithUserPass()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, "", AUTH_USER, "ZZ")
	expectResult(t, c, errRe)
}

func TestPasswordClientGoodConnect(t *testing.T) {
	s := runAuthServerWithUserPass()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, "", AUTH_USER, AUTH_PASS)
	expectResult(t, c, okRe)
}

////////////////////////////////////////////////////////////
// The bcrypt username/password version
////////////////////////////////////////////////////////////

// Generated with util/mkpasswd
const BCRYPT_AUTH_PASS = "#00L2zPr!j11VsT@e9QGPt"
const BCRYPT_AUTH_HASH = "$2a$11$wDaOBnEx0GbcFTOzJRywpexI/dxH3sqV3.adFefmDDMJggnOTNqKS"

func runAuthServerWithBcryptUserPass() *server.Server {
	opts := DefaultTestOptions
	opts.Port = AUTH_PORT
	opts.Username = AUTH_USER
	opts.Password = BCRYPT_AUTH_HASH

	auth := &auth.Plain{Username: AUTH_USER, Password: BCRYPT_AUTH_HASH}
	return RunServerWithAuth(&opts, auth)
}

func TestBadBcryptPassword(t *testing.T) {
	s := runAuthServerWithBcryptUserPass()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, "", AUTH_USER, BCRYPT_AUTH_HASH)
	expectResult(t, c, errRe)
}

func TestGoodBcryptPassword(t *testing.T) {
	s := runAuthServerWithBcryptUserPass()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, "", AUTH_USER, BCRYPT_AUTH_PASS)
	expectResult(t, c, okRe)
}

////////////////////////////////////////////////////////////
// The bcrypt authorization token version
////////////////////////////////////////////////////////////

const BCRYPT_AUTH_TOKEN = "743&@WeTlIwtHDytI5Bnxl"
const BCRYPT_AUTH_TOKEN_HASH = "$2a$11$Gp5x2rvdzfm9rUREuyQeBOFd61oPYKoLWSI2fJN7DAFMF34Z9o4s2"

func runAuthServerWithBcryptToken() *server.Server {
	opts := DefaultTestOptions
	opts.Port = AUTH_PORT
	opts.Authorization = BCRYPT_AUTH_TOKEN_HASH
	return RunServerWithAuth(&opts, &auth.Token{Token: BCRYPT_AUTH_TOKEN_HASH})
}

func TestBadBcryptToken(t *testing.T) {
	s := runAuthServerWithBcryptToken()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, BCRYPT_AUTH_TOKEN_HASH, "", "")
	expectResult(t, c, errRe)
}

func TestGoodBcryptToken(t *testing.T) {
	s := runAuthServerWithBcryptToken()
	defer s.Shutdown()
	c := createClientConn(t, "localhost", AUTH_PORT)
	defer c.Close()
	expectAuthRequired(t, c)
	doAuthConnect(t, c, BCRYPT_AUTH_TOKEN, "", "")
	expectResult(t, c, okRe)
}
