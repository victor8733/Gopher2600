// This file is part of Gopher2600.
//
// Gopher2600 is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Gopher2600 is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Gopher2600.  If not, see <https://www.gnu.org/licenses/>.

package hiscore

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/jetsetilly/gopher2600/errors"
)

// SetServer to use for hiscore storage
func SetServer(input io.Reader, output io.Writer, server string) error {
	// get reference to hiscore preferences
	prefs, err := newPreferences()
	if err != nil {
		return errors.New(errors.HiScore, err)
	}

	// server has not been provided so prompt for it
	if server == "" {
		output.Write([]byte("Enter server: "))
		var b []byte
		b = make([]byte, 255)
		_, err := input.Read(b)
		if err != nil {
			return errors.New(errors.HiScore, err)
		}
		server = string(b)
	}

	// crop newline
	server = strings.Split(server, "\n")[0]

	// parse entered url
	url, err := url.Parse(server)
	if err != nil {
		return errors.New(errors.HiScore, err)
	}

	// error on path, but allow a single slash (by removing it)
	if url.Path != "" {
		if url.Path != "/" {
			return errors.New(errors.HiScore, "do not include path in server setting")
		}
	}

	// nillify all fields aside from schema and host
	url.Path = ""
	url.RawPath = ""
	url.Fragment = ""
	url.RawQuery = ""
	url.ForceQuery = false
	url.Opaque = ""
	url.User = nil

	// update server setting and save changes
	prefs.Server.Set(url.String())
	return prefs.Save()
}

// Login prepares the authentication token for the hiscore server
func Login(input io.Reader, output io.Writer, username string) error {
	// get reference to hiscore preferences
	prefs, err := newPreferences()
	if err != nil {
		return errors.New(errors.HiScore, err)
	}

	// we can't login unless highscore server has been specified
	if prefs.Server.Get() == "" {
		return errors.New(errors.HiScore, "no highscore server available")
	}

	// prompt for username if it has not been supplied
	if strings.TrimSpace(username) == "" {
		output.Write([]byte("Enter username: "))
		var b []byte
		b = make([]byte, 255)
		_, err := input.Read(b)
		if err != nil {
			return errors.New(errors.HiScore, err)
		}
		username = strings.Split(string(b), "\n")[0]
	}

	// prompt for password
	//
	// !!TODO: noecho hiscore server password
	output.Write([]byte("(WARNING: password will be visible)\n"))
	output.Write([]byte("Enter password: "))
	var b []byte
	b = make([]byte, 255)
	_, err = input.Read(b)
	if err != nil {
		return errors.New(errors.HiScore, err)
	}
	password := strings.Split(string(b), "\n")[0]

	// send login request to server
	var cl http.Client
	data := url.Values{"username": {username}, "password": {password}}
	resp, err := cl.PostForm(fmt.Sprintf("%s/rest-auth/login/", prefs.Server), data)
	if err != nil {
		return errors.New(errors.HiScore, err)
	}

	// get response
	response, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.New(errors.HiScore, err)
	}

	// unmarshal response
	var key map[string]string
	err = json.Unmarshal(response, &key)
	if err != nil {
		return errors.New(errors.HiScore, err)
	}

	// update authentication key and save changes
	prefs.AuthToken.Set(key["key"])
	return prefs.Save()
}

// Logoff forgets the authentication token for the hiscore server
func Logoff() error {
	// get reference to hiscore preferences
	prefs, err := newPreferences()
	if err != nil {
		return errors.New(errors.HiScore, err)
	}

	// blank authentication key and save changes
	prefs.AuthToken.Set("")
	return prefs.Save()
}
