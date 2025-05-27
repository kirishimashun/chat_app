package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func UploadImage(w http.ResponseWriter, r *http.Request) {
	// 最大10MB
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "フォームデータの解析に失敗しました", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "画像ファイルが必要です", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 保存先ディレクトリ
	saveDir := "./uploads"
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		http.Error(w, "保存ディレクトリの作成に失敗しました", http.StatusInternalServerError)
		return
	}

	// ユニークなファイル名を作成
	fileName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(handler.Filename))
	savePath := filepath.Join(saveDir, fileName)

	// ファイル保存
	dst, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "画像ファイルの保存に失敗しました", http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "画像の書き込みに失敗しました", http.StatusInternalServerError)
		return
	}

	// 公開用URLを生成
	url := fmt.Sprintf("http://localhost:8080/static/%s", fileName)

	// JSONでURLを返す
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

// package handlers

// import (
// 	"fmt"
// 	"io"
// 	"net/http"
// 	"os"
// 	"path/filepath"
// )

// func UploadImage(w http.ResponseWriter, r *http.Request) {
// 	r.ParseMultipartForm(10 << 20) // 最大10MB

// 	file, handler, err := r.FormFile("image")
// 	if err != nil {
// 		http.Error(w, "画像ファイルが必要です", http.StatusBadRequest)
// 		return
// 	}
// 	defer file.Close()

// 	saveDir := "./uploads"
// 	os.MkdirAll(saveDir, 0755)

// 	fileName := fmt.Sprintf("%d_%s", r.ContentLength, handler.Filename)
// 	savePath := filepath.Join(saveDir, fileName)

// 	dst, err := os.Create(savePath)
// 	if err != nil {
// 		http.Error(w, "保存エラー", http.StatusInternalServerError)
// 		return
// 	}
// 	defer dst.Close()
// 	io.Copy(dst, file)

// 	// ✅ 絶対URLに変更する
// 	url := fmt.Sprintf("http://localhost:8080/static/%s", fileName)
// 	w.Header().Set("Content-Type", "application/json")
// 	w.Write([]byte(fmt.Sprintf(`{"url": "%s"}`, url)))
// }
