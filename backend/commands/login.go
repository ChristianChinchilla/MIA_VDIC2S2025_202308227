package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

func ExecuteLogin(user, pass, id string) {
	//verificar que no haya sesion activa
	if IsSessionActive() {
		fmt.Println("Error: Ya existe una sesión activa. Use 'logout' para cerrar la sesión actual.")
		return
	}

	//validar parametros obligatorios
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

	//buscar la particion montada por id
	mounted := GetMountedPartition(id)
	if mounted == nil {
		fmt.Printf("Error: No se encontró ninguna partición montada con ID '%s'.\n", id)
		return
	}

	usersContent, err := readUsersFile(mounted)
	if err != nil {
		fmt.Printf("Error al leer archivo users.txt: %v\n", err)
		return
	}

	//buscar el usuario en users.txt
	userGroup, found := findUser(usersContent, user, pass)
	if !found {
		fmt.Printf("Error: Usuario '%s' no encontrado o contraseña incorrecta.\n", user)
		return
	}

	//iniciar sesion
	StartSession(user, userGroup, id)

	fmt.Printf("Sesión iniciada exitosamente.\n")
	fmt.Printf("   Usuario: %s\n", user)
	fmt.Printf("   Grupo: %s\n", userGroup)
	fmt.Printf("   Partición: %s\n", id)
}

// leer el archivo users.txt del sistema de archivos
func readUsersFile(mounted *MountedPartition) (string, error) {
	file, err := os.OpenFile(mounted.Path, os.O_RDWR, 0644)
	if err != nil {
		return "", fmt.Errorf("error al abrir el disco: %v", err)
	}
	defer file.Close()

	//leer el mbr para obtener informacion de la particion
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		return "", fmt.Errorf("error al leer el MBR: %v", err)
	}

	//encontrar la particion especifica
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
		return "", fmt.Errorf("no se pudo encontrar la particion '%s'", mounted.Name)
	}

	//leer el superbloque
	file.Seek(partition.Part_start, 0)
	var superblock structs.SuperBloque
	if err := binary.Read(file, binary.LittleEndian, &superblock); err != nil {
		return "", fmt.Errorf("error al leer el superbloque: %v", err)
	}
	inodePosition := superblock.S_inode_start + (1 * superblock.S_inode_s)

	file.Seek(inodePosition, 0)
	var usersInode structs.Inodos
	if err := binary.Read(file, binary.LittleEndian, &usersInode); err != nil {
		return "", fmt.Errorf("error al leer el inodo de users.txt: %v", err)
	}

	var allContent []byte

	for i := 0; i < 15; i++ {
		if usersInode.I_block[i] == -1 {
			break
		}

		blockPosition := superblock.S_block_start + (usersInode.I_block[i] * superblock.S_block_s)
		file.Seek(blockPosition, 0)

		var block structs.BloqueArchivo
		if err := binary.Read(file, binary.LittleEndian, &block); err != nil {
			return "", fmt.Errorf("error al leer bloque %d: %v", i, err)
		}

		allContent = append(allContent, block.BContent[:]...)
	}

	var content string
	if usersInode.I_s > 0 && usersInode.I_s <= int64(len(allContent)) {
		content = string(allContent[:usersInode.I_s])
	} else {
		content = string(allContent)
	}

	return content, nil
}

// buscar usuario en el contenido de users.txt
func findUser(usersContent, user, pass string) (string, bool) {
	lines := strings.Split(usersContent, "\n")

	//primero encontrar todos los grupos para hacer el mapeo
	groups := make(map[string]string)
	groupsByName := make(map[string]string)

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

			if tipo == "G" && gid != "0" {
				groups[gid] = groupName
				groupsByName[groupName] = gid
			}
		}
	}

	//ahora buscar el usuario
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 5 {
			uid := strings.TrimSpace(parts[0])
			tipo := strings.TrimSpace(parts[1])
			gid := strings.TrimSpace(parts[2])
			username := strings.TrimSpace(parts[3])
			password := strings.TrimSpace(parts[4])

			if tipo == "U" && uid != "0" { //es un usuario y no esta eliminado
				if username == user && password == pass {
					var groupName string

					if foundName, exists := groups[gid]; exists {
						groupName = foundName
					} else {

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

// comando logout
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
