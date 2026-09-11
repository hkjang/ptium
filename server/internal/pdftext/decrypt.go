package pdftext

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
)

// A PDF somebody protected is still a PDF anybody can open.
//
// The usual setting in an office is an owner password with no user password:
// the file asks nothing of a reader, it only says printing or copying is not
// allowed. Its streams are encrypted all the same, so reading one without
// undoing that gives nothing — and a page with nothing on it was reported to
// the person as a file with no text in it, "a scan". It was a report the whole
// time, with a tick box set on it.
//
// A file that really does want a password is a different thing, and is said to
// be one.

// padding is the constant string the format pads a password with.
var padding = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56,
	0xFF, 0xFA, 0x01, 0x08, 0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
	0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

// decryption is how a file's streams are unlocked, once.
type decryption struct {
	key []byte
	aes bool
	// wholeFile marks a file whose key does not change from object to object,
	// which is what the newest revisions do.
	wholeFile bool
}

// unlock reads the file's own encryption dictionary and works out the key for
// an empty user password. It reports whether the file can be read at all.
func (d *document) unlock(encrypt dict, firstID []byte) (*decryption, bool) {
	if d.name(encrypt["Filter"]) != "Standard" {
		return nil, false
	}
	revision, _ := d.number(encrypt["R"])
	version, _ := d.number(encrypt["V"])
	length, ok := d.number(encrypt["Length"])
	if !ok {
		length = 40
	}
	owner, _ := d.resolve(encrypt["O"]).([]byte)
	user, _ := d.resolve(encrypt["U"]).([]byte)
	permissions, _ := d.number(encrypt["P"])

	if revision >= 5 {
		return unlockModern(owner, user, d.resolve(encrypt["OE"]), d.resolve(encrypt["UE"]))
	}
	if len(owner) < 32 {
		return nil, false
	}
	// Algorithm 2, with the empty user password.
	digest := md5.New()
	digest.Write(padding)
	digest.Write(owner[:32])
	var allowed [4]byte
	binary.LittleEndian.PutUint32(allowed[:], uint32(int32(permissions)))
	digest.Write(allowed[:])
	digest.Write(firstID)
	if revision >= 4 {
		if metadata, ok := d.resolve(encrypt["EncryptMetadata"]).(bool); ok && !metadata {
			digest.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
		}
	}
	key := digest.Sum(nil)
	size := int(length) / 8
	if revision == 2 {
		size = 5
	}
	if size < 5 || size > 16 {
		size = 16
	}
	if revision >= 3 {
		for round := 0; round < 50; round++ {
			again := md5.Sum(key[:size])
			key = again[:]
		}
	}
	key = key[:size]

	held := &decryption{key: key}
	if version >= 4 {
		held.aes = d.usesAES(encrypt)
	}
	// The key is right only if it reproduces what the file says about it. A
	// file that wants a real password fails here, and is told apart from one
	// that has no text in it.
	if !openableWith(held, revision, user, firstID, permissions) {
		return nil, false
	}
	return held, true
}

// usesAES reads the crypt filter the file names for its streams.
func (d *document) usesAES(encrypt dict) bool {
	filters := d.dict(encrypt["CF"])
	chosen := d.name(encrypt["StmF"])
	if chosen == "" {
		chosen = "StdCF"
	}
	if found := d.dict(filters[chosen]); found != nil {
		method := d.name(found["CFM"])
		return method == "AESV2" || method == "AESV3"
	}
	return false
}

// openableWith checks the key against the /U the file carries, which is what
// tells a file anybody can open from one that wants a password.
func openableWith(held *decryption, revision float64, user, firstID []byte, permissions float64) bool {
	if len(user) < 16 {
		return false
	}
	if revision == 2 {
		cipher, err := rc4.NewCipher(held.key)
		if err != nil {
			return false
		}
		said := make([]byte, 32)
		cipher.XORKeyStream(said, padding)
		return bytes.Equal(said, user[:32])
	}
	digest := md5.New()
	digest.Write(padding)
	digest.Write(firstID)
	said := digest.Sum(nil)
	for round := 0; round <= 19; round++ {
		turned := make([]byte, len(held.key))
		for at := range held.key {
			turned[at] = held.key[at] ^ byte(round)
		}
		cipher, err := rc4.NewCipher(turned)
		if err != nil {
			return false
		}
		cipher.XORKeyStream(said, said)
	}
	return bytes.Equal(said[:16], user[:16])
}

// unlockModern reads the 256-bit revisions, where an empty user password is
// checked against a hash the file carries and the key is wrapped beside it.
func unlockModern(owner, user []byte, ownerKey, userKey value) (*decryption, bool) {
	wrapped, ok := userKey.([]byte)
	if !ok || len(user) < 48 || len(wrapped) < 32 {
		return nil, false
	}
	salt, keySalt := user[32:40], user[40:48]
	if !bytes.Equal(hash2B(nil, salt, nil), user[:32]) {
		return nil, false
	}
	intermediate := hash2B(nil, keySalt, nil)
	block, err := aes.NewCipher(intermediate)
	if err != nil {
		return nil, false
	}
	key := make([]byte, 32)
	cipher.NewCBCDecrypter(block, make([]byte, 16)).CryptBlocks(key, wrapped[:32])
	return &decryption{key: key, aes: true, wholeFile: true}, true
}

// hash2B is the password hash the 256-bit revisions use. The first round is a
// plain SHA-256; later revisions keep stirring until the work settles.
func hash2B(password, salt, extra []byte) []byte {
	first := sha256.New()
	first.Write(password)
	first.Write(salt)
	first.Write(extra)
	key := first.Sum(nil)
	for round := 0; ; round++ {
		repeated := bytes.Repeat(append(append(append([]byte{}, password...), key...), extra...), 64)
		block, err := aes.NewCipher(key[:16])
		if err != nil {
			return key
		}
		encrypted := make([]byte, len(repeated))
		cipher.NewCBCEncrypter(block, key[16:32]).CryptBlocks(encrypted, repeated)
		sum := 0
		for _, one := range encrypted[:16] {
			sum += int(one)
		}
		switch sum % 3 {
		case 0:
			said := sha256.Sum256(encrypted)
			key = said[:]
		case 1:
			said := sha512.Sum384(encrypted)
			key = said[:]
		default:
			said := sha512.Sum512(encrypted)
			key = said[:]
		}
		if round >= 63 && int(encrypted[len(encrypted)-1]) <= round-32 {
			return key[:32]
		}
	}
}

// forObject is the key one object's data is encrypted with. Before the 256-bit
// revisions every object has its own, worked out from the file's key and the
// object's number.
func (held *decryption) forObject(number, generation int) []byte {
	if held.wholeFile {
		return held.key
	}
	digest := md5.New()
	digest.Write(held.key)
	digest.Write([]byte{byte(number), byte(number >> 8), byte(number >> 16),
		byte(generation), byte(generation >> 8)})
	if held.aes {
		digest.Write([]byte{0x73, 0x41, 0x6C, 0x54})
	}
	size := len(held.key) + 5
	if size > 16 {
		size = 16
	}
	return digest.Sum(nil)[:size]
}

// decrypt undoes one object's encryption.
func (held *decryption) decrypt(data []byte, number, generation int) []byte {
	key := held.forObject(number, generation)
	if !held.aes {
		stream, err := rc4.NewCipher(key)
		if err != nil {
			return data
		}
		said := make([]byte, len(data))
		stream.XORKeyStream(said, data)
		return said
	}
	// AES writes the initial block in front of what it protects.
	if len(data) <= aes.BlockSize || len(data)%aes.BlockSize != 0 {
		return nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	said := make([]byte, len(data)-aes.BlockSize)
	cipher.NewCBCDecrypter(block, data[:aes.BlockSize]).CryptBlocks(said, data[aes.BlockSize:])
	if padded := int(said[len(said)-1]); padded >= 1 && padded <= aes.BlockSize && padded <= len(said) {
		said = said[:len(said)-padded]
	}
	return said
}
