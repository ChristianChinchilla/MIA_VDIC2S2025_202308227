package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ExecuteRmdisk(diskName string) {
	// Asegurar que el nombre tenga la extensión .mia
	if !strings.HasSuffix(strings.ToLower(diskName), ".mia") {
		diskName += ".mia"
	}

	// Construir la ruta completa usando el directorio de discos
	fullPath := filepath.Join(DisksDirectory, diskName)

	// Verificar que el archivo existe antes de eliminarlo
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		fmt.Printf("Error: El archivo '%s' no existe.\n", diskName)
		return
	}

	if err := os.Remove(fullPath); err != nil {
		fmt.Printf("Error al eliminar el archivo: %v\n", err)
		return
	}

	fmt.Printf("Disco eliminado exitosamente: '%s'.\n", diskName)
}
