package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

func ExecuteCat(files map[string]string) {
	// Verificar sesión activa
	if !RequireActiveSession() {
		return
	}

	// Validar que se proporcione al menos un archivo
	if len(files) == 0 {
		fmt.Println("Error: debe especificar al menos un archivo con -file1=...")
		return
	}

	// Obtener sesión actual
	session := GetCurrentSession()
	if session == nil {
		fmt.Println("Error: No se pudo obtener la sesión actual.")
		return
	}

	// Buscar la partición montada de la sesión
	mounted := GetMountedPartition(session.PartitionID)
	if mounted == nil {
		fmt.Printf("Error: No se encontró la partición montada con ID '%s'.\n", session.PartitionID)
		return
	}

	fmt.Println("Contenido de los archivos:")
	fmt.Println("========================================")

	// Procesar archivos en orden (file1, file2, file3, ...)
	for i := 1; i <= len(files); i++ {
		fileKey := fmt.Sprintf("file%d", i)
		filePath, exists := files[fileKey]

		if !exists {
			continue
		}

		if filePath == "" {
			fmt.Printf("Error: el parámetro -%s está vacío.\n", fileKey)
			continue
		}

		// Mostrar separador entre archivos (excepto el primero)
		if i > 1 {
			fmt.Println("----------------------------------------")
		}

		fmt.Printf("Archivo: %s\n", filePath)

		// Leer el archivo del sistema de archivos EXT2
		content, err := readFileFromEXT2WithPermissions(mounted, filePath, session.User)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		// Mostrar contenido
		fmt.Print(content)
		if !strings.HasSuffix(content, "\n") {
			fmt.Println() // Agregar salto de línea si no existe
		}
	}

	fmt.Println("========================================")
}

// Leer archivo del sistema EXT2 con verificación de permisos
func readFileFromEXT2WithPermissions(mounted *MountedPartition, filePath string, currentUser string) (string, error) {
	// Abrir el archivo del disco
	file, err := os.OpenFile(mounted.Path, os.O_RDWR, 0644)
	if err != nil {
		return "", fmt.Errorf("error al abrir el disco: %v", err)
	}
	defer file.Close()

	// Obtener partición y superbloque
	_, superblock, err := getPartitionAndSuperblock(file, mounted)
	if err != nil {
		return "", err
	}

	// ← CAMBIO: Usar navegación por rutas completas
	inodeIndex, err := findFileAtPath(file, superblock, filePath)
	if err != nil {
		return "", fmt.Errorf("archivo '%s' no encontrado", filePath)
	}

	// Leer el inodo del archivo
	inodePosition := superblock.S_inode_start + (inodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var fileInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &fileInode); err != nil {
		return "", fmt.Errorf("error al leer el inodo del archivo: %v", err)
	}

	// Verificar que es un archivo (no directorio)
	if fileInode.I_type != '1' {
		return "", fmt.Errorf("'%s' no es un archivo", filePath)
	}

	// Verificar permisos de lectura
	if !hasReadPermission(&fileInode, currentUser) {
		return "", fmt.Errorf("sin permisos de lectura para el archivo '%s'", filePath)
	}

	// Leer el contenido del archivo usando función multi-bloque
	content, err := readFileContentMultiBlock(file, superblock, &fileInode)
	if err != nil {
		return "", fmt.Errorf("error al leer el contenido del archivo: %v", err)
	}

	return content, nil
}

// Verificar permisos de lectura
func hasReadPermission(inode *structs.Inodos, currentUser string) bool {
	// Para root, siempre tiene permisos
	if currentUser == "root" {
		return true
	}

	// Obtener permisos del archivo (formato: rwx para propietario, grupo, otros)
	permissions := string(inode.I_perm[:])

	// Verificar formato de permisos (debe ser como "644", "755", etc.)
	if len(permissions) != 3 {
		return false
	}

	// El primer dígito son los permisos del propietario
	ownerPerms := permissions[0]

	// Verificar si el propietario tiene permiso de lectura
	// En octal: 4 = lectura, 6 = lectura+escritura, 7 = lectura+escritura+ejecución
	if ownerPerms >= '4' {
		return true
	}

	return false
}

func findFileAtPath(file *os.File, superblock *structs.SuperBloque, filePath string) (int64, error) {
	// Parsear la ruta del archivo
	parsedPath := parsePath(filePath)
	if parsedPath == nil {
		return -1, fmt.Errorf("ruta inválida: %s", filePath)
	}

	// Si no es absoluta, buscar en directorio raíz (compatibilidad)
	if !parsedPath.IsAbsolute {
		return findFileInRootDirectory(file, superblock, filePath)
	}

	// Navegar hasta el directorio padre
	currentInodeIndex := int64(0) // Empezar desde el directorio raíz

	for _, dirName := range parsedPath.Directories {
		nextInode, err := findInodeInDirectory(file, superblock, currentInodeIndex, dirName)
		if err != nil {
			return -1, fmt.Errorf("directorio '%s' no encontrado", dirName)
		}
		currentInodeIndex = nextInode
	}

	// Buscar el archivo en el directorio final
	fileInode, err := findInodeInDirectory(file, superblock, currentInodeIndex, parsedPath.FileName)
	if err != nil {
		return -1, fmt.Errorf("archivo '%s' no encontrado", parsedPath.FileName)
	}

	return fileInode, nil
}
