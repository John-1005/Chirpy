package main

import (
    "encoding/json"
    "net/http"
)



func checkChirpHandler(w http.ResponseWriter, r *http.Request) {
  type message struct {
    Body string `json:"body"`
  }


  type validate struct {
    Valid bool `json:"valid"`
  }

  decoder := json.NewDecoder(r.Body)
  msg := message{}
  err := decoder.Decode(&msg)

  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "Couldn't decode message", err)
    return
  }

  const maxLength = 140

  if len(msg.Body) > maxLength {
    respondWithError(w, http.StatusBadRequest, "Chirp is too long", nil )
    return
  }

  respondWithJSON(w, http.StatusOK, validate{
    Valid: true,
  })
}
