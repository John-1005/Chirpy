package auth

import (
   "testing"
   "time"

   "github.com/google/uuid"
)


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
      name: "No password",
      password: "",
      hash: hash2,
      wantErr: true,
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
    t.Run(tt.name, func(t *testing.T){
      err := CheckPasswordHash(tt.password, tt.hash)
      if (err != nil) != tt.wantErr {
        t.Errorf("CheckPasswordHash(), error = %v, wantErr %v", err, tt.wantErr)
      }
    })
  }

}

func TestAndValidateJWT(t *testing.T) {
  userID := uuid.New()

  token,
}

