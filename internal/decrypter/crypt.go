package decrypter

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"

	"github.com/njupg/pdf/internal/types"
)

func New(password string, encrypt types.Dict, id string) (*Decrypter, error) {
	n, _ := encrypt["Length"].(int64)
	if n == 0 {
		n = 40
	}
	v, _ := encrypt["V"].(int64)
	r, _ := encrypt["R"].(int64)
	o, _ := encrypt["O"].(string)
	u, _ := encrypt["U"].(string)
	p, _ := encrypt["P"].(int64)
	P := uint32(p)

	if n%8 != 0 || n < 40 || (n > 128 && n != 256) {
		return nil, fmt.Errorf("malformed PDF: %d-bit encryption key", n)
	}
	if !validateVersion(v, encrypt) {
		return nil, fmt.Errorf("unsupported PDF: encryption version V=%d; %v", v, encrypt)
	}

	if r < 2 || r == 5 || r > 6 {
		return nil, fmt.Errorf("malformed PDF: encryption revision R=%d", r)
	}

	pw := []byte(password)

	if r == 6 {
		ue := encrypt["UE"].(string)
		perms := encrypt["Perms"].(string)
		return newR6(pw, []byte(u), []byte(ue), []byte(perms))
	}

	if len(o) != 32 || len(u) != 32 {
		return nil, fmt.Errorf("malformed PDF: missing O= or U= encryption parameters")
	}

	// TODO: Password should be converted to Latin-1.
	h := md5.New()
	if len(pw) >= 32 {
		h.Write(pw[:32])
	} else {
		h.Write(pw)
		h.Write(passwordPad[:32-len(pw)])
	}
	h.Write([]byte(o))
	h.Write([]byte{byte(P), byte(P >> 8), byte(P >> 16), byte(P >> 24)})
	h.Write([]byte(id))
	key := h.Sum(nil)

	if r >= 3 {
		for i := 0; i < 50; i++ {
			h.Reset()
			h.Write(key[:n/8])
			key = h.Sum(key[:0])
		}
		key = key[:n/8]
	} else {
		key = key[:40/8]
	}

	c, err := rc4.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("malformed PDF: invalid RC4 key: %v", err)
	}

	var w []byte
	if r == 2 {
		w = make([]byte, 32)
		copy(w, passwordPad)
		c.XORKeyStream(w, w)
	} else {
		h.Reset()
		h.Write(passwordPad)
		h.Write([]byte(id))
		w = h.Sum(nil)
		c.XORKeyStream(w, w)

		for i := 1; i <= 19; i++ {
			key1 := make([]byte, len(key))
			copy(key1, key)
			for j := range key1 {
				key1[j] ^= byte(i)
			}
			c, _ = rc4.NewCipher(key1)
			c.XORKeyStream(w, w)
		}
	}

	if !bytes.HasPrefix([]byte(u), w) {
		return nil, ErrInvalidPassword
	}

	return &Decrypter{key: key, v: int(v)}, nil
}

var passwordPad = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

var ErrInvalidPassword = fmt.Errorf("encrypted PDF: invalid password")

func newR6(password, u, ue, perms []byte) (*Decrypter, error) {
	if len(password) > 127 {
		password = password[:127]
	}
	if len(u) < 48 {
		return nil, fmt.Errorf("bad r6 U(%d)", len(u))
	}
	u = u[:48]

	if !bytes.Equal(hashR6(password, u[32:40]), u[:32]) {
		return nil, errors.New("can't determine user key")
	}

	intermediate := hashR6(password, u[40:48])
	b, err := aes.NewCipher([]byte(intermediate))
	if err != nil {
		return nil, err
	}
	var iv [16]byte
	cbc := cipher.NewCBCDecrypter(b, iv[:])
	key := make([]byte, 32)
	cbc.CryptBlocks(key, []byte(ue))

	dec := make([]byte, 16)
	b, err = aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	b.Decrypt(dec, []byte(perms))
	if string(dec[9:12]) != "adb" {
		return nil, errors.New("params didn't validate")
	}

	return &Decrypter{key: key, v: 5}, nil
}

// hashR6 implements Algorithm 2.B of ISO32000-2.
func hashR6(p, salt []byte) []byte {
	h := sha256.New()
	h.Write(p)
	h.Write(salt)
	k := h.Sum(nil)

	for i := 1; ; i++ {
		k1 := bytes.Repeat(append(p, k...), 64)
		b, err := aes.NewCipher(k[:16])
		if err != nil {
			panic(err)
		}
		enc := cipher.NewCBCEncrypter(b, k[16:32])
		e := make([]byte, len(k1))
		enc.CryptBlocks(e, k1)

		var mod int
		for i := 0; i < 16; i++ {
			mod += int(e[i])
		}
		switch mod % 3 {
		case 0:
			v := sha256.Sum256(e)
			k = v[:]
		case 1:
			v := sha512.Sum384(e)
			k = v[:]
		case 2:
			v := sha512.Sum512(e)
			k = v[:]
		}

		if i >= 64 && e[len(e)-1] <= byte(i-32) {
			break
		}
	}

	return k[:32]
}

type Decrypter struct {
	key []byte
	v   int
}

func (d *Decrypter) aes() bool { return d.v == 4 || d.v == 5 }

func (d *Decrypter) Decrypt(ptr types.Objptr, rd io.Reader) (io.Reader, error) {
	if d == nil {
		return rd, nil
	}

	key := d.cryptKey(ptr)
	if d.aes() {
		cb, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("bad AES key: %w", err)
		}
		iv := make([]byte, 16)
		io.ReadFull(rd, iv)
		cbc := cipher.NewCBCDecrypter(cb, iv)
		rd = &cbcReader{cbc: cbc, rd: rd, buf: make([]byte, 16)}
	} else {
		c, _ := rc4.NewCipher(key)
		rd = &cipher.StreamReader{S: c, R: rd}
	}
	return rd, nil
}

func (d *Decrypter) cryptKey(ptr types.Objptr) []byte {
	if d.v == 5 {
		return d.key
	}

	h := md5.New()
	h.Write(d.key)
	h.Write([]byte{byte(ptr.ID), byte(ptr.ID >> 8), byte(ptr.ID >> 16), byte(ptr.Gen), byte(ptr.Gen >> 8)})
	if d.v == 4 {
		h.Write([]byte("sAlT"))
	}
	return h.Sum(nil)
}

type cbcReader struct {
	cbc  cipher.BlockMode
	rd   io.Reader
	buf  []byte
	pend []byte
}

func (r *cbcReader) Read(b []byte) (n int, err error) {
	if len(r.pend) == 0 {
		_, err = io.ReadFull(r.rd, r.buf)
		if err != nil {
			return 0, err
		}
		r.cbc.CryptBlocks(r.buf, r.buf)
		r.pend = r.buf
	}
	n = copy(b, r.pend)
	r.pend = r.pend[n:]
	return n, nil
}

func validateVersion(v int64, encrypt types.Dict) bool {
	switch v {
	case 1, 2:
		return true
	case 4, 5: // validate params below.
	default:
		return false
	}

	cf, ok := encrypt["CF"].(types.Dict)
	if !ok {
		return false
	}
	stmf, ok := encrypt["StmF"].(types.Name)
	if !ok {
		return false
	}
	strf, ok := encrypt["StrF"].(types.Name)
	if !ok {
		return false
	}
	if stmf != strf {
		return false
	}
	cfparam := cf[stmf].(types.Dict)
	if cfparam["AuthEvent"] != nil && cfparam["AuthEvent"] != types.Name("DocOpen") {
		return false
	}

	len := int64(16)
	cfm := types.Name("AESV2")
	if v == 5 {
		len = 32
		cfm = types.Name("AESV3")
	}
	if cfparam["Length"] != nil && cfparam["Length"] != len {
		return false
	}
	if cfparam["CFM"] != cfm {
		return false
	}
	return true
}
