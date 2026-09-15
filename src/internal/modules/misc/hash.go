package misc

import (
	"crypto/md5" //nolint:gosec // non-cryptographic hashing (cache keys / content digests), not security-sensitive
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// BcryptHash Use bcrypt for encryption
func BcryptHash(password string) string {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

// BcryptCheck Compare the plain password with the hashed password in DB
func BcryptCheck(plainPassword, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword))
	return err == nil
}

// MD5Hash MD5 encryption, optional salt can be provided
func MD5Hash(str []byte, salt ...byte) string {
	h := md5.New() //nolint:gosec // see import: non-cryptographic use
	h.Write(str)
	if len(salt) > 0 {
		h.Write(salt)
	}
	return hex.EncodeToString(h.Sum(nil))
}
