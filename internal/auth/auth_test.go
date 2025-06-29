package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

var testSecret = "testing-secret-token"

func TestCheckPasswordHash(t *testing.T) {
	password1 := "passwordtest123"
	password2 := "secondpasswordtest098"
	hash1, _ := HashPassword(password1)
	hash2, _ := HashPassword(password2)

	tests := []struct {
		name     string
		password string
		hash     string
		wantErr  bool
	}{
		{
			name:     "Correct Password",
			password: password1,
			hash:     hash1,
			wantErr:  false,
		},
		{
			name:     "Incorrect Password",
			password: "badpassword",
			hash:     hash1,
			wantErr:  true,
		},
		{
			name:     "Correct Password 2",
			password: password2,
			hash:     hash2,
			wantErr:  false,
		},
		{
			name:     "No password",
			password: "",
			hash:     hash2,
			wantErr:  true,
		},
		{
			name:     "Password doesn't match hash",
			password: "doesntmatch",
			hash:     hash2,
			wantErr:  true,
		},

		{
			name:     "Wrong hash",
			password: password1,
			hash:     "wronghash",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckPasswordHash(tt.password, tt.hash)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckPasswordHash(), error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

}

func TestAndValidateJWT(t *testing.T) {
	userID := uuid.New()
	expiresIn := time.Hour

	token, err := MakeJWT(userID, testSecret, expiresIn)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	validatedID, err := ValidateJWT(token, testSecret)
	if err != nil {
		t.Fatalf("ValidatedJWT failed: %v", err)
	}

	if validatedID != userID {
		t.Errorf("Expected user ID: %v, got %v", userID, validatedID)
	}
}

func TestValidateJWT_InvalidToken(t *testing.T) {
	_, err := ValidateJWT("not.a.valid.token", testSecret)
	if err == nil {
		t.Fatal("Expected error for invalid token, got none")
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	userID := uuid.New()
	token, err := MakeJWT(userID, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT failed: %v", err)
	}

	_, err = ValidateJWT(token, "wrong-secret")
	if err == nil {
		t.Fatal("Expected error when validating wrong secret: received none")
	}
}

func TestValidatedJWT_ExpiredToken(t *testing.T) {
	userID := uuid.New()

	token, err := MakeJWT(userID, testSecret, -1*time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT failed: %v", err)
	}

	_, err = ValidateJWT(token, testSecret)
	if err == nil {
		t.Fatal("Expected error for expired token, received none")
	}
}

func TestGetBearerToken(t *testing.T) {

	tests := []struct {
		name      string
		headers   http.Header
		wantToken string
		wantErr   bool
	}{
		{
			name:      "Valid Bearer Token",
			headers:   http.Header{"Authorization": []string{"Bearer mytoken123"}},
			wantToken: "mytoken123",
			wantErr:   false,
		},
		{
			name:    "Missing Authorization Header",
			headers: http.Header{},
			wantErr: true,
		},
		{
			name:    "Wrong Prefix",
			headers: http.Header{"Authorization": []string{"Basic abcdef"}},
			wantErr: true,
		},
		{
			name:    "Empty Token",
			headers: http.Header{"Authorization": []string{"Bearer "}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotToken, err := GetBearerToken(tt.headers)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetBearerToken() error = %v, want %v", err, tt.wantErr)
			}
			if gotToken != tt.wantToken {
				t.Errorf("GetBearerToken() = %v, want %v", gotToken, tt.wantToken)
			}
		})
	}
}
