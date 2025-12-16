package commands

import (
	"fmt"
	"os"
	"strings"
)

func ExecuteRmdisk(path string) {
    if !strings.HasSuffix(strings.ToLower(path), ".mia") {
        path += ".mia"
    }

    // Verificar que el archivo existe antes de eliminarlo
    if _, err := os.Stat(path); os.IsNotExist(err) {
        fmt.Printf("Error: El archivo '%s' no existe.\n", path)
        return
    }

    if err := os.Remove(path); err != nil {
        fmt.Printf("Error al eliminar el archivo: %v\n", err)
        return
    }

    fmt.Printf("Disco eliminado exitosamente en '%s'.\n", path)
}