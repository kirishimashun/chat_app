package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func UploadImage(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 最大10MB

	file, handler, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "画像ファイルが必要です", http.StatusBadRequest)
		return
	}
	defer file.Close()

	saveDir := "./uploads"
	os.MkdirAll(saveDir, 0755)

	fileName := fmt.Sprintf("%d_%s", r.ContentLength, handler.Filename)
	savePath := filepath.Join(saveDir, fileName)

	dst, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "保存エラー", http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	io.Copy(dst, file)

	// ✅ 絶対URLに変更する
	url := fmt.Sprintf("http://localhost:8080/static/%s", fileName)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"url": "%s"}`, url)))
}
