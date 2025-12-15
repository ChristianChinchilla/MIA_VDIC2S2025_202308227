package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

func ExecuteLogin(user, pass, id string) {
	// Verificar que no haya sesión activa
	if IsSessionActive() {
		fmt.Println("Error: Ya existe una sesión activa. Use 'logout' para cerrar la sesión actual.")
		return
	}

	// Validar parámetros obligatorios
	if user == "" {
		fmt.Println("Error: el parámetro -user es obligatorio para login.")
		return
	}

	if pass == "" {
		fmt.Println("Error: el parámetro -pass es obligatorio para login.")
		return
	}

	if id == "" {
		fmt.Println("Error: el parámetro -id es obligatorio para login.")
		return
	}

	// Buscar la partición montada por ID
	mounted := GetMountedPartition(id)
	if mounted == nil {
		fmt.Printf("Error: No se encontró ninguna partición montada con ID '%s'.\n", id)
		return
	}

	// Leer el archivo users.txt del sistema de archivos
	usersContent, err := readUsersFile(mounted)
	if err != nil {
		fmt.Printf("Error al leer archivo users.txt: %v\n", err)
		return
	}

	// Buscar el usuario en users.txt
	userGroup, found := findUser(usersContent, user, pass)
	if !found {
		fmt.Printf("Error: Usuario '%s' no encontrado o contraseña incorrecta.\n", user)
		return
	}

	// Iniciar sesión
	StartSession(user, userGroup, id)

	fmt.Printf("Sesión iniciada exitosamente.\n")
	fmt.Printf("   Usuario: %s\n", user)
	fmt.Printf("   Grupo: %s\n", userGroup)
	fmt.Printf("   Partición: %s\n", id)
}

// Leer el archivo users.txt del sistema de archivos
func readUsersFile(mounted *MountedPartition) (string, error) {
	// Abrir el archivo del disco
	file, err := os.OpenFile(mounted.Path, os.O_RDWR, 0644)
	if err != nil {
		return "", fmt.Errorf("error al abrir el disco: %v", err)
	}
	defer file.Close()

	// Leer el MBR para obtener información de la partición
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		return "", fmt.Errorf("error al leer el MBR: %v", err)
	}

	// Encontrar la partición específica
	var partition *structs.Partition
	for _, p := range mbr.Mbr_partitions {
		if p.Part_status != '0' {
			partitionNameBytes := p.Part_name[:]
			nullIndex := len(partitionNameBytes)
			for j, b := range partitionNameBytes {
				if b == 0 {
					nullIndex = j
					break
				}
			}
			partitionName := strings.TrimSpace(string(partitionNameBytes[:nullIndex]))

			if strings.EqualFold(partitionName, mounted.Name) || partitionName == mounted.Name {
				partitionCopy := p
				partition = &partitionCopy
				break
			}
		}
	}

	if partition == nil {
		return "", fmt.Errorf("no se pudo encontrar la partición '%s'", mounted.Name)
	}

	// Leer el superbloque
	file.Seek(partition.Part_start, 0)
	var superblock structs.SuperBloque
	if err := binary.Read(file, binary.LittleEndian, &superblock); err != nil {
		return "", fmt.Errorf("error al leer el superbloque: %v", err)
	}

	// Leer el inodo de users.txt (inodo 1)
	inodePosition := superblock.S_inode_start + (1 * superblock.S_inode_s)

	file.Seek(inodePosition, 0)
	var usersInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &usersInode); err != nil {
		return "", fmt.Errorf("error al leer el inodo de users.txt: %v", err)
	}

	// Leer múltiples bloques si es necesario
	var allContent []byte

	for i := 0; i < 15; i++ { // Máximo 15 bloques directos
		if usersInode.I_block[i] == -1 {
			break // No hay más bloques
		}

		blockPosition := superblock.S_block_start + (usersInode.I_block[i] * superblock.S_block_s)
		file.Seek(blockPosition, 0)

		var block structs.BloqueArchivo
		if err := binary.Read(file, binary.LittleEndian, &block); err != nil {
			return "", fmt.Errorf("error al leer bloque %d: %v", i, err)
		}

		allContent = append(allContent, block.BContent[:]...)
	}

	// Usar el tamaño del inodo para obtener el contenido exacto
	var content string
	if usersInode.I_s > 0 && usersInode.I_s <= int64(len(allContent)) {
		content = string(allContent[:usersInode.I_s])
	} else {
		content = string(allContent)
	}

	return content, nil
}

// Buscar usuario en el contenido de users.txt
func findUser(usersContent, user, pass string) (string, bool) {
	lines := strings.Split(usersContent, "\n")

	// Primero, encontrar todos los grupos para hacer el mapeo
	groups := make(map[string]string)       // GID -> Nombre del grupo
	groupsByName := make(map[string]string) // Nombre -> GID

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			gid := strings.TrimSpace(parts[0])
			tipo := strings.TrimSpace(parts[1])
			groupName := strings.TrimSpace(parts[2])

			if tipo == "G" && gid != "0" { // Es un grupo y no está eliminado
				groups[gid] = groupName       // GID numérico -> nombre
				groupsByName[groupName] = gid // nombre -> GID numérico
			}
		}
	}

	// Ahora buscar el usuario
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 5 {
			uid := strings.TrimSpace(parts[0])
			tipo := strings.TrimSpace(parts[1])
			gid := strings.TrimSpace(parts[2]) // Este puede ser número o nombre
			username := strings.TrimSpace(parts[3])
			password := strings.TrimSpace(parts[4])

			if tipo == "U" && uid != "0" { // Es un usuario y no está eliminado
				if username == user && password == pass {
					// Buscar el nombre del grupo
					var groupName string

					// Caso 1: gid es numérico (buscar por GID)
					if foundName, exists := groups[gid]; exists {
						groupName = foundName
					} else {
						// Caso 2: gid es el nombre del grupo directamente
						if _, exists := groupsByName[gid]; exists {
							groupName = gid
						} else {
							groupName = "unknown"
						}
					}

					return groupName, true
				}
			}
		}
	}

	return "", false
}

// Comando LOGOUT
func ExecuteLogout() {
	if !IsSessionActive() {
		fmt.Println("Error: No hay sesión activa para cerrar.")
		return
	}

	session := GetCurrentSession()
	fmt.Printf("Cerrando sesión del usuario '%s'.\n", session.User)
	EndSession()
	fmt.Println("Sesión cerrada exitosamente.")
}
