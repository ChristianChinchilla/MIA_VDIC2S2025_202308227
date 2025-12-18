package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

func ExecuteCat(files map[string]string) {
	//verificar sesion activa
	if !RequireActiveSession() {
		return
	}

	//validar que se proporcione al menos un archivo
	if len(files) == 0 {
		fmt.Println("Error: debe especificar al menos un archivo con -file1=...")
		return
	}

	//sesion actual
	session := GetCurrentSession()
	if session == nil {
		fmt.Println("Error: No se pudo obtener la sesión actual.")
		return
	}

	//buscar la particion
	mounted := GetMountedPartition(session.PartitionID)
	if mounted == nil {
		fmt.Printf("Error: No se encontró la partición montada con ID '%s'.\n", session.PartitionID)
		return
	}

	fmt.Println("Contenido de los archivos:")
	fmt.Println("========================================")

	//procesar archivos en orden
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

		//mostrar separador entre archivos
		if i > 1 {
			fmt.Println("----------------------------------------")
		}

		fmt.Printf("Archivo: %s\n", filePath)

		//leer el archivo del sistema de archivos
		content, err := readFileFromEXT2WithPermissions(mounted, filePath, session.User)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		//mostrar contenido
		fmt.Print(content)
		if !strings.HasSuffix(content, "\n") {
			fmt.Println()
		}
	}

	fmt.Println("========================================")
}

// leer archivo del sistema
func readFileFromEXT2WithPermissions(mounted *MountedPartition, filePath string, currentUser string) (string, error) {
	//abrir el archivo del disco
	file, err := os.OpenFile(mounted.Path, os.O_RDWR, 0644)
	if err != nil {
		return "", fmt.Errorf("error al abrir el disco: %v", err)
	}
	defer file.Close()

	//obtener particion
	_, superblock, err := getPartitionAndSuperblock(file, mounted)
	if err != nil {
		return "", err
	}

	//navegacion por rutas completas
	inodeIndex, err := findFileAtPath(file, superblock, filePath)
	if err != nil {
		return "", fmt.Errorf("archivo '%s' no encontrado", filePath)
	}

	//leer el inodo del archivo
	inodePosition := superblock.S_inode_start + (inodeIndex * superblock.S_inode_s)
	file.Seek(inodePosition, 0)
	var fileInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &fileInode); err != nil {
		return "", fmt.Errorf("error al leer el inodo del archivo: %v", err)
	}

	//verificar que es un archivo
	if fileInode.I_type != '1' {
		return "", fmt.Errorf("'%s' no es un archivo", filePath)
	}

	//verificar permisos de lectura
	if !hasReadPermission(&fileInode, currentUser) {
		return "", fmt.Errorf("sin permisos de lectura para el archivo '%s'", filePath)
	}

	//leer el contenido del archivo
	content, err := readFileContentMultiBlock(file, superblock, &fileInode)
	if err != nil {
		return "", fmt.Errorf("error al leer el contenido del archivo: %v", err)
	}

	return content, nil
}

// verificar permisos de lectura
func hasReadPermission(inode *structs.Inodos, currentUser string) bool {
	//permisos del root
	if currentUser == "root" {
		return true
	}

	//obtener permisos del archivo
	permissions := string(inode.I_perm[:])

	//verificar formato de permisos
	if len(permissions) != 3 {
		return false
	}

	//el primer digito
	ownerPerms := permissions[0]

	//verificar si el propietario tiene permiso de lectura
	if ownerPerms >= '4' {
		return true
	}

	return false
}

func findFileAtPath(file *os.File, superblock *structs.SuperBloque, filePath string) (int64, error) {
	//parsear
	parsedPath := parsePath(filePath)
	if parsedPath == nil {
		return -1, fmt.Errorf("ruta inválida: %s", filePath)
	}

	//si no es absoluta
	if !parsedPath.IsAbsolute {
		return findFileInRootDirectory(file, superblock, filePath)
	}

	//navegar hasta el directorio padre
	currentInodeIndex := int64(0)

	for _, dirName := range parsedPath.Directories {
		nextInode, err := findInodeInDirectory(file, superblock, currentInodeIndex, dirName)
		if err != nil {
			return -1, fmt.Errorf("directorio '%s' no encontrado", dirName)
		}
		currentInodeIndex = nextInode
	}

	//buscar el archivo en el directorio final
	fileInode, err := findInodeInDirectory(file, superblock, currentInodeIndex, parsedPath.FileName)
	if err != nil {
		return -1, fmt.Errorf("archivo '%s' no encontrado", parsedPath.FileName)
	}

	return fileInode, nil
}
