package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
)

// نفس الهيكل الموجود في الـ Controller
type Request struct {
	Command       string `json:"command"`
	Wallpaper     string `json:"wallpaper,omitempty"`
	WallpaperName string `json:"wallpaper_name,omitempty"`
	WallpaperData string `json:"wallpaper_data,omitempty"`
	FileName      string `json:"file_name,omitempty"`
	FileData      string `json:"file_data,omitempty"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func main() {
	// لو هتشغل الـ Agent على جهاز تاني، غير "localhost" للـ IP بتاع جهاز الـ Controller
	conn, err := net.Dial("tcp", "localhost:9000")
	if err != nil {
		log.Fatal("Could not connect to controller:", err)
	}
	defer conn.Close()

	// إرسال رسالة تعريفية (Handshake) للسيرفر
	handshake := map[string]string{"name": "Agent-PC-1"}
	json.NewEncoder(conn).Encode(handshake)
	fmt.Println("Connected to Controller successfully!")

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	// انتظار الأوامر من الـ Controller
	for {
		var req Request
		if err := decoder.Decode(&req); err != nil {
			log.Println("Connection closed by controller.")
			return
		}

		fmt.Println("Received command:", req.Command)
		resp := handleRequest(req)
		encoder.Encode(resp)
	}
}

func handleRequest(req Request) Response {
	switch req.Command {
	case "lock":
		exec.Command("rundll32.exe", "user32.dll,LockWorkStation").Run()
		return Response{OK: true, Message: "Machine locked"}

	case "shutdown":
		exec.Command("shutdown", "/s", "/t", "0").Run()
		return Response{OK: true, Message: "Machine shutting down"}

	case "wallpaper":
		data, err := base64.StdEncoding.DecodeString(req.WallpaperData)
		if err != nil {
			return Response{OK: false, Message: "Failed to decode image data"}
		}

		// حفظ الصورة في ملف مؤقت
		tmpPath := filepath.Join(os.TempDir(), req.WallpaperName)
		err = os.WriteFile(tmpPath, data, 0644)
		if err != nil {
			return Response{OK: false, Message: "Failed to save image"}
		}

		// تغيير الخلفية في الويندوز
		exec.Command("reg", "add", "HKEY_CURRENT_USER\\Control Panel\\Desktop", "/v", "Wallpaper", "/t", "REG_SZ", "/d", tmpPath, "/f").Run()
		exec.Command("RUNDLL32.EXE", "user32.dll,UpdatePerUserSystemParameters").Run()

		return Response{OK: true, Message: "Wallpaper changed to " + req.WallpaperName}

	case "send_file":
		data, err := base64.StdEncoding.DecodeString(req.FileData)
		if err != nil {
			return Response{OK: false, Message: "Failed to decode file data"}
		}

		// حفظ الملف في نفس مسار تشغيل الـ Agent مع إضافة كلمة received_
		savePath := filepath.Join(".", "received_"+req.FileName)
		err = os.WriteFile(savePath, data, 0644)
		if err != nil {
			return Response{OK: false, Message: err.Error()}
		}

		return Response{OK: true, Message: "File saved as: " + savePath}

	default:
		return Response{OK: false, Message: "Unknown command"}
	}
}