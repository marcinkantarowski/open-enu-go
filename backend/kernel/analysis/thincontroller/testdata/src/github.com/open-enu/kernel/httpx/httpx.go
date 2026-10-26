package httpx

import "net/http"

func WriteError(w http.ResponseWriter, r *http.Request, err error) {}
