package main

import (
    "encoding/json"
    "net/http"
    "strings"
)



func handlerCheckChirp(w http.ResponseWriter, r *http.Request) {
  type message struct {
    Body string `json:"body"`
  }


  type ChirpResponse struct {
    CleanedBody string `json:"cleaned_body"`
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

  cleanedMessage := chirpFilter(msg.Body)

  response := ChirpResponse{CleanedBody: cleanedMessage}
  respondWithJSON(w, http.StatusOK, response)
}



func chirpFilter(message string) string {

  words := strings.Split(message, " ")
  
  var cleanedWords []string

  bannedWords := map[string]struct{} {
    "kerfuffle": {},
    "sharbert": {},
    "fornax": {},
  }

  for _, w := range words {

    lowered := strings.ToLower(w)
    if _, banned := bannedWords[lowered]; banned {
      cleanedWords = append(cleanedWords, "****")
    }else{
      cleanedWords = append(cleanedWords, w)
    }

  }

  cleanedMessage := strings.Join(cleanedWords, " ")
  return cleanedMessage

}
