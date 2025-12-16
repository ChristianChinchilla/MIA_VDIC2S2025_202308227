package commands

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
	"backend/structs"
)


func ExecuteMkdisk(size int, unit string, fit string, path string) {
	var diskSize int64

	unit = strings.ToUpper(unit)

	switch unit {
		case "K":
			diskSize = int64(size) * 1024
		case "M", "":
			diskSize = int64(size) * 1024 * 1024
		default:
			fmt.Printf("Error: Unidad invalida '%s'. Use 'K' o 'M'.\n", unit)
			return
	}

	if diskSize <= 0 {
		fmt.Println("Error: El parámetro -size debe ser mayor que 0.")
		return
	}

	var fitByte byte

	fit = strings.ToUpper(fit)

	switch fit {
		case "BF":
			fitByte = 'b'
		case "WF":
			fitByte = 'w'
		case "FF", "":
			fitByte = 'f'
		default:
			fmt.Printf("Error: Ajuste '%s' no válido. Use 'BF', 'WF' o 'FF'.\n", fit)
			return
	}

	if !strings.HasSuffix(strings.ToLower(path), ".mia"){
		path += ".mia"
	}

	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Error al crear el archivo: %v\n", err)
		return
	}

	file, err := os.Create(path)
	if err != nil {
		fmt.Printf("Error al crear el archivo: %v\n", err)
		return
	}

	defer file.Close()

	chunk := make([]byte, 1024)

	for i := int64(0); i < diskSize/1024; i++ {
		if _, err := file.Write(chunk); err != nil {
			fmt.Printf("Error al escribir en el archivo: %v\n", err)
			return
		}
	}

	if err := file.Truncate(diskSize); err != nil {
		fmt.Printf("Error al truncar el archivo: %v\n", err)
		return
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	diskSignature := r.Int63()

	mbr := structs.NewMBR(diskSize, fitByte, diskSignature)

	file.Seek(0, 0)
	if err := binary.Write(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("Error al escribir el MBR: %v\n", err)
		return
	}

	fmt.Printf("Disco creado exitosamente en '%s' con tamaño %d bytes, ajuste '%s' y firma %d.\n", path, diskSize, fit, diskSignature)

}