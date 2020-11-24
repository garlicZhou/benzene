package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	p2p_crypto "github.com/libp2p/go-libp2p-core/crypto"
	"io"
	"os"
	"sync"
)

var lock sync.Mutex

// PrivKeyStore is used to persist private key to/from file
type PrivKeyStore struct {
	Key string `json:"key"`
}

// Unmarshal is a function that unmarshals the data from the
// reader into the specified value.
func Unmarshal(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

// Marshal is a function that marshals the object into an
// io.Reader.
func Marshal(v interface{}) (io.Reader, error) {
	b, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// GenKeyP2PRand generates a pair of RSA keys used in libp2p host, using random seed
func GenKeyP2PRand() (p2p_crypto.PrivKey, p2p_crypto.PubKey, error) {
	return p2p_crypto.GenerateKeyPair(p2p_crypto.RSA, 2048)
}

// Save saves a representation of v to the file at path.
func Save(path string, v interface{}) error {
	lock.Lock()
	defer lock.Unlock()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r, err := Marshal(v)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	return err
}

// Load loads the file at path into v.
func Load(path string, v interface{}) error {
	lock.Lock()
	defer lock.Unlock()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
	}
	defer f.Close()
	return Unmarshal(f, v)
}

// LoadPrivateKey parses the key string in base64 format and return PrivKey
func LoadPrivateKey(key string) (p2p_crypto.PrivKey, p2p_crypto.PubKey, error) {
	if key != "" {
		k1, err := p2p_crypto.ConfigDecodeKey(key)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode key: %v", err)
		}
		priKey, err := p2p_crypto.UnmarshalPrivateKey(k1)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal private key: %v", err)
		}
		pubKey := priKey.GetPublic()
		return priKey, pubKey, nil
	}
	return nil, nil, fmt.Errorf("empty key string")
}

// SavePrivateKey convert the PrivKey to base64 format and return string
func SavePrivateKey(key p2p_crypto.PrivKey) (string, error) {
	if key != nil {
		b, err := p2p_crypto.MarshalPrivateKey(key)
		if err != nil {
			return "", fmt.Errorf("failed to marshal private key: %v", err)
		}
		str := p2p_crypto.ConfigEncodeKey(b)
		return str, nil
	}
	return "", fmt.Errorf("key is nil")
}

// SaveKeyToFile save private key to keyfile
func SaveKeyToFile(keyfile string, key p2p_crypto.PrivKey) (err error) {
	str, err := SavePrivateKey(key)
	if err != nil {
		return
	}

	keyStruct := PrivKeyStore{Key: str}

	err = Save(keyfile, &keyStruct)
	return
}

// LoadKeyFromFile load private key from keyfile
// If the private key is not loadable or no file, it will generate
// a new random private key
func LoadKeyFromFile(keyfile string) (key p2p_crypto.PrivKey, pk p2p_crypto.PubKey, err error) {
	var keyStruct PrivKeyStore
	err = Load(keyfile, &keyStruct)
	if err != nil {
		Logger().Info().
			Str("keyfile", keyfile).
			Msg("No private key can be loaded from file")
		Logger().Info().Msg("Using random private key")
		key, pk, err = GenKeyP2PRand()
		if err != nil {
			Logger().Error().
				AnErr("GenKeyP2PRand Error", err).
				Msg("LoadedKeyFromFile")
			panic(err)
		}
		err = SaveKeyToFile(keyfile, key)
		if err != nil {
			Logger().Error().
				AnErr("keyfile", err).
				Msg("failed to save key to keyfile")
		}
		return key, pk, nil
	}
	key, pk, err = LoadPrivateKey(keyStruct.Key)
	return key, pk, err
}