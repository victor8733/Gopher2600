// This file is part of Gopher2600.
//
// Gopher2600 is free software: you can redistribute it and/or modify
// it under the terms of the gnu general public license as published by
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

package cartridgeloader

import (
	"crypto/sha1"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/jetsetilly/gopher2600/errors"
)

// Loader is used to specify the cartridge to use when Attach()ing to
// the VCS. it also permits the called to specify the mapping of the cartridge
// (if necessary. fingerprinting is pretty good)
type Loader struct {

	// filename of cartridge to load.
	Filename string

	// empty string or "AUTO" indicates automatic fingerprinting
	Mapping string

	// expected hash of the loaded cartridge. empty string indicates that the
	// hash is unknown and need not be validated. after a load operation the
	// value will be the hash of the loaded data
	Hash string

	// copy of the loaded data. subsequence calls to Load() will return a copy
	// of this data
	data []byte
}

// NewLoader is the preferred method of initialisation for the Loader type.
//
// The mapping argument will be used to set the Mapping field, unless the
// argument is either "AUTO" or the empty string. In which case the file
// extension is used to set the field.
//
// File extensions should be the same as the ID of the intended mapper, as
// defined in the cartridge package. The exception is the DPC+ format which
// requires the file extension "DP+"
//
// File extensions ".BIN" and "A26" will set the Mapping field to "AUTO".
//
// Alphabetic characters in file extensions can be in upper or lower case or a
// mixture of both.
func NewLoader(filename string, mapping string) Loader {
	cl := Loader{
		Filename: filename,
		Mapping:  "AUTO",
	}

	mapping = strings.TrimSpace(strings.ToUpper(mapping))
	if mapping != "AUTO" && mapping != "" {
		cl.Mapping = mapping
	} else {
		ext := strings.ToUpper(path.Ext(filename))
		switch ext {
		case ".BIN":
			fallthrough
		case ".A26":
			cl.Mapping = "AUTO"
		case ".2k":
			fallthrough
		case ".4k":
			fallthrough
		case ".F8":
			fallthrough
		case ".F6":
			fallthrough
		case ".F4":
			fallthrough
		case ".2k+":
			fallthrough
		case ".4k+":
			fallthrough
		case ".F8+":
			fallthrough
		case ".F6+":
			fallthrough
		case ".F4+":
			fallthrough
		case ".FA":
			fallthrough
		case ".FE":
			fallthrough
		case ".E0":
			fallthrough
		case ".E7":
			fallthrough
		case ".3F":
			fallthrough
		case ".AR":
			fallthrough
		case ".DPC":
			cl.Mapping = ext[1:]
		case "DP+":
			cl.Mapping = "DPC+"
		}
	}

	return cl
}

// ShortName returns a shortened version of the CartridgeLoader filename
func (cl Loader) ShortName() string {
	shortCartName := path.Base(cl.Filename)
	shortCartName = strings.TrimSuffix(shortCartName, path.Ext(cl.Filename))
	return shortCartName
}

// HasLoaded returns true if Load() has been successfully called
func (cl Loader) HasLoaded() bool {
	return len(cl.data) > 0
}

// Load the cartridge data and return as a byte array. Loader filenames with a
// valid schema will use that method to load the data. Currently supported
// schemes are HTTP and local files.
func (cl *Loader) Load() ([]byte, error) {
	if len(cl.data) > 0 {
		return cl.data[:], nil
	}

	url, err := url.Parse(cl.Filename)
	if err != nil {
		return nil, errors.New(errors.CartridgeLoader, err)
	}

	switch url.Scheme {
	case "http":
		resp, err := http.Get(cl.Filename)
		if err != nil {
			return nil, errors.New(errors.CartridgeLoader, err)
		}
		defer resp.Body.Close()

		size := resp.ContentLength

		cl.data = make([]byte, size)
		_, err = resp.Body.Read(cl.data)
		if err != nil {
			return nil, errors.New(errors.CartridgeLoader, err)
		}

	case "file":
		fallthrough

	case "":
		f, err := os.Open(cl.Filename)
		if err != nil {
			return nil, errors.New(errors.CartridgeLoader, err)
		}
		defer f.Close()

		// get file info. not using Stat() on the file handle because the
		// windows version (when running under wine) does not handle that
		cfi, err := os.Stat(cl.Filename)
		if err != nil {
			return nil, errors.New(errors.CartridgeLoader, err)
		}
		size := cfi.Size()

		cl.data = make([]byte, size)
		_, err = f.Read(cl.data)
		if err != nil {
			return nil, errors.New(errors.CartridgeLoader, err)
		}

	default:
		return nil, errors.New(errors.CartridgeLoader, fmt.Sprintf("unsupported URL scheme (%s)", url.Scheme))
	}

	// generate hash
	hash := fmt.Sprintf("%x", sha1.Sum(cl.data))

	// check for hash consistency
	if cl.Hash != "" && cl.Hash != hash {
		return nil, errors.New(errors.CartridgeLoader, "unexpected hash value")
	}

	// not generated hash
	cl.Hash = hash

	return cl.data[:], nil
}
