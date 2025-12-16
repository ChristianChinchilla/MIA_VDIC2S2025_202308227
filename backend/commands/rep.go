package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DiskSegment struct {
	Type        string
	Name        string
	Label       string
	Details     string
	Tooltip     string
	StartBytes  int64
	SizeBytes   int64
	StartStr    string
	SizeStr     string
	Percentage  float64
	CSSClass    string
	LegendClass string
	Status      string
}

// AGREGAR función auxiliar:
func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 bytes"
	}

	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d bytes", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB"}
	if exp >= len(units) {
		return fmt.Sprintf("%d bytes", bytes)
	}

	return fmt.Sprintf("%.2f %s", float64(bytes)/float64(div), units[exp])
}

// calculateDiskStructure calcula la estructura del disco para visualización
func calculateDiskStructure(mbr structs.MBR, totalSize int64) []DiskSegment {
	var segments []DiskSegment
	currentPos := int64(0)

	// MBR (primer sector, 512 bytes típicamente)
	mbrSize := int64(512) // Tamaño estándar del MBR
	segments = append(segments, DiskSegment{
		Type:        "MBR",
		Name:        "Master Boot Record",
		Label:       "MBR",
		Details:     formatBytes(mbrSize),
		Tooltip:     "Master Boot Record - Sector de arranque",
		StartBytes:  0,
		SizeBytes:   mbrSize,
		StartStr:    "0",
		SizeStr:     formatBytes(mbrSize),
		Percentage:  float64(mbrSize) / float64(totalSize) * 100,
		CSSClass:    "segment-mbr",
		LegendClass: "legend-mbr",
		Status:      "Sistema",
	})
	currentPos = mbrSize

	// Obtener particiones ordenadas por posición de inicio
	var partitions []structs.Partition
	for _, part := range mbr.Mbr_partitions {
		if part.Part_status != 0 && part.Part_start > 0 && part.Part_s > 0 {
			partitions = append(partitions, part)
		}
	}

	// Ordenar particiones por posición de inicio
	for i := 0; i < len(partitions)-1; i++ {
		for j := i + 1; j < len(partitions); j++ {
			if partitions[i].Part_start > partitions[j].Part_start {
				partitions[i], partitions[j] = partitions[j], partitions[i]
			}
		}
	}

	// Procesar cada partición
	for _, partition := range partitions {
		// Espacio libre antes de la partición
		if partition.Part_start > currentPos {
			freeSize := partition.Part_start - currentPos
			segments = append(segments, DiskSegment{
				Type:        "Libre",
				Name:        "Espacio Libre",
				Label:       "Libre",
				Details:     formatBytes(freeSize),
				Tooltip:     fmt.Sprintf("Espacio libre: %s", formatBytes(freeSize)),
				StartBytes:  currentPos,
				SizeBytes:   freeSize,
				StartStr:    fmt.Sprintf("%d", currentPos),
				SizeStr:     formatBytes(freeSize),
				Percentage:  float64(freeSize) / float64(totalSize) * 100,
				CSSClass:    "segment-free",
				LegendClass: "legend-free",
				Status:      "Disponible",
			})
		}

		// Partición actual
		name := strings.TrimSpace(string(partition.Part_name[:]))
		if name == "" {
			name = "Sin nombre"
		}

		var segmentType, label, cssClass, legendClass string
		if partition.Part_type == 'E' || partition.Part_type == 'e' {
			segmentType = "Extendida"
			label = "Extendida"
			cssClass = "segment-extended"
			legendClass = "legend-extended"
		} else {
			segmentType = "Primaria"
			label = "Primaria"
			cssClass = "segment-primary"
			legendClass = "legend-primary"
		}

		status := "Activa"
		if partition.Part_status == '0' || partition.Part_status == 0 {
			status = "Inactiva"
		}

		segments = append(segments, DiskSegment{
			Type:        segmentType,
			Name:        name,
			Label:       label,
			Details:     formatBytes(partition.Part_s),
			Tooltip:     fmt.Sprintf("%s: %s - %s", segmentType, name, formatBytes(partition.Part_s)),
			StartBytes:  partition.Part_start,
			SizeBytes:   partition.Part_s,
			StartStr:    fmt.Sprintf("%d", partition.Part_start),
			SizeStr:     formatBytes(partition.Part_s),
			Percentage:  float64(partition.Part_s) / float64(totalSize) * 100,
			CSSClass:    cssClass,
			LegendClass: legendClass,
			Status:      status,
		})

		// Si es partición extendida, agregar particiones lógicas
		if partition.Part_type == 'E' || partition.Part_type == 'e' {
			// TODO: Implementar lectura de EBRs para particiones lógicas
			// Por ahora, las particiones lógicas se mostrarán como parte de la extendida
		}

		currentPos = partition.Part_start + partition.Part_s
	}

	// Espacio libre al final
	if currentPos < totalSize {
		freeSize := totalSize - currentPos
		segments = append(segments, DiskSegment{
			Type:        "Libre",
			Name:        "Espacio Libre",
			Label:       "Libre",
			Details:     formatBytes(freeSize),
			Tooltip:     fmt.Sprintf("Espacio libre al final: %s", formatBytes(freeSize)),
			StartBytes:  currentPos,
			SizeBytes:   freeSize,
			StartStr:    fmt.Sprintf("%d", currentPos),
			SizeStr:     formatBytes(freeSize),
			Percentage:  float64(freeSize) / float64(totalSize) * 100,
			CSSClass:    "segment-free",
			LegendClass: "legend-free",
			Status:      "Disponible",
		})
	}

	return segments
}

// ExecuteRep genera reportes con Graphviz
func ExecuteRep(name string, path string, id string, pathFileLs string) {
	// Validar parámetros obligatorios
	if name == "" {
		fmt.Println("❌ Error: El parámetro -name es obligatorio")
		return
	}
	if path == "" {
		fmt.Println("❌ Error: El parámetro -path es obligatorio")
		return
	}
	if id == "" {
		fmt.Println("❌ Error: El parámetro -id es obligatorio")
		return
	}

	// Validar tipos de reporte válidos
	validReports := []string{"mbr", "disk", "inode", "block", "bm_inode", "bm_block", "tree", "sb", "file", "ls"}
	name = strings.ToLower(name)
	isValid := false
	for _, valid := range validReports {
		if name == valid {
			isValid = true
			break
		}
	}

	if !isValid {
		fmt.Printf("❌ Error: Tipo de reporte '%s' no válido. Tipos válidos: %s\n", name, strings.Join(validReports, ", "))
		return
	}

	// Obtener información de la partición montada
	mountedPartition := GetMountedPartition(id)
	if mountedPartition == nil {
		fmt.Printf("❌ Error: No se encontró una partición montada con ID '%s'\n", id)
		return
	}

	// Crear directorio si no existe
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("❌ Error al crear directorio: %v\n", err)
		return
	}

	// Generar el reporte según el tipo
	switch name {
	case "mbr":
		generateMBRReport(mountedPartition.Path, path)
	case "disk":
		generateDiskReport(mountedPartition.Path, path)
	case "inode":
		generateInodeReport(mountedPartition, path)
	case "block":
		generateBlockReport(mountedPartition, path)
	case "bm_inode":
		generateBitmapInodeReport(mountedPartition, path)
	case "bm_block":
		generateBitmapBlockReport(mountedPartition, path)
	case "tree":
		generateTreeReport(mountedPartition, path)
	case "sb":
		generateSuperBlockReport(mountedPartition, path)
	case "file":
		generateFileReport(mountedPartition, path, pathFileLs)
	case "ls":
		generateLsReport(mountedPartition, path, pathFileLs)
	}
}

// generateMBRReport genera el reporte del MBR
func generateMBRReport(diskPath string, outputPath string) {
	file, err := os.Open(diskPath)
	if err != nil {
		fmt.Printf("❌ Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	// Leer MBR con orden de bytes mixto
	mbr, err := readMBRMixed(file)
	if err != nil {
		fmt.Printf("❌ Error al leer el MBR: %v\n", err)
		return
	}

	htmlContent := generateMBRHTML(mbr, diskPath)
	generateHTMLReport(htmlContent, outputPath, "MBR")
}

// AGREGAR función nueva para lectura mixta:
func readMBRMixed(file *os.File) (structs.MBR, error) {
	var mbr structs.MBR

	file.Seek(0, 0)
	buffer := make([]byte, 1024)
	_, err := file.Read(buffer)
	if err != nil {
		return mbr, err
	}

	// Leer campos del MBR principal con LittleEndian
	mbr.Mbr_tamano = int64(binary.LittleEndian.Uint64(buffer[0:8]))
	mbr.Mbr_fecha_creacion = int64(binary.LittleEndian.Uint64(buffer[8:16]))
	mbr.Mbr_dsk_signature = int64(binary.LittleEndian.Uint64(buffer[16:24]))
	mbr.Dsk_fit = buffer[24]

	// Leer particiones con LittleEndian (como estaba funcionando)
	file.Seek(0, 0)
	var mbrTemp structs.MBR
	binary.Read(file, binary.LittleEndian, &mbrTemp)

	// Copiar solo las particiones que funcionaban bien
	mbr.Mbr_partitions = mbrTemp.Mbr_partitions

	return mbr, nil
}

func readSuperBlockMixed(file *os.File, partitionStart int64) (structs.SuperBloque, error) {
	var superblock structs.SuperBloque

	// Posicionarse en el inicio de la partición
	file.Seek(partitionStart, 0)

	// Leer buffer del superbloque (1024 bytes para estar seguros)
	buffer := make([]byte, 1024)
	_, err := file.Read(buffer)
	if err != nil {
		return superblock, err
	}

	// Primero intentar con LittleEndian (método original)
	file.Seek(partitionStart, 0)
	if err := binary.Read(file, binary.LittleEndian, &superblock); err != nil {
		return superblock, err
	}

	// Validaciones de consistencia
	if superblock.S_inodes_count <= 0 || superblock.S_inodes_count > 1000000 {
		return superblock, fmt.Errorf("número de inodos inválido: %d", superblock.S_inodes_count)
	}

	if superblock.S_inode_s <= 0 || superblock.S_inode_s > 1024 {
		return superblock, fmt.Errorf("tamaño de inodo inválido: %d", superblock.S_inode_s)
	}

	if superblock.S_block_s <= 0 || superblock.S_block_s > 8192 {
		return superblock, fmt.Errorf("tamaño de bloque inválido: %d", superblock.S_block_s)
	}

	// Verificar que los contadores sean consistentes
	if superblock.S_free_inodes_count > superblock.S_inodes_count {
		fmt.Printf("⚠️  Advertencia: inodos libres (%d) > total (%d)\n",
			superblock.S_free_inodes_count, superblock.S_inodes_count)
	}

	return superblock, nil
}

// generateDiskReport genera el reporte del disco
func generateDiskReport(diskPath string, outputPath string) {
	file, err := os.Open(diskPath)
	if err != nil {
		fmt.Printf("❌ Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Printf("❌ Error al obtener información del archivo: %v\n", err)
		return
	}

	// Usar la misma función mixta que en MBR
	mbr, err := readMBRMixed(file)
	if err != nil {
		fmt.Printf("❌ Error al leer el MBR: %v\n", err)
		return
	}

	htmlContent := generateDiskHTML(mbr, diskPath, fileInfo)
	generateHTMLReport(htmlContent, outputPath, "DISK")
}

// generateSuperBlockReport genera el reporte del superbloque
func generateSuperBlockReport(partition *MountedPartition, outputPath string) {
	file, err := os.Open(partition.Path)
	if err != nil {
		fmt.Printf("❌ Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	// Leer MBR para encontrar la partición
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("❌ Error al leer el MBR: %v\n", err)
		return
	}

	var partitionStart int64
	found := false

	// Normalizar el nombre de la partición montada
	mountedName := strings.TrimSpace(strings.TrimRight(partition.Name, "\x00"))

	for _, part := range mbr.Mbr_partitions {
		if part.Part_status != '0' && part.Part_status != 0 {
			partitionName := strings.TrimSpace(strings.TrimRight(string(part.Part_name[:]), "\x00"))

			if partitionName == mountedName {
				partitionStart = part.Part_start
				found = true
				break
			}
		}
	}

	if !found {
		fmt.Printf("❌ Error: No se encontró la partición '%s'\n", mountedName)
		return
	}

	// Leer superbloque
	superblock, err := readSuperBlockMixed(file, partitionStart)
	if err != nil {
		fmt.Printf("❌ Error al leer el superbloque: %v\n", err)
		return
	}

	htmlContent := generateSuperBlockHTML(superblock, partition.Name, partition.Path)
	generateHTMLReport(htmlContent, outputPath, "SUPERBLOCK")
}

// generateInodeReport genera el reporte de inodos
func generateInodeReport(partition *MountedPartition, outputPath string) {

	file, err := os.Open(partition.Path)
	if err != nil {
		fmt.Printf("❌ Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	// Leer MBR para encontrar la partición
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("❌ Error al leer el MBR: %v\n", err)
		return
	}

	var partitionStart int64
	found := false

	// Normalizar el nombre de la partición montada (quitar espacios y null bytes)
	mountedName := strings.TrimSpace(strings.TrimRight(partition.Name, "\x00"))

	for _, part := range mbr.Mbr_partitions {
		if part.Part_status != '0' && part.Part_status != 0 {
			// Normalizar el nombre de la partición del MBR
			partitionName := strings.TrimSpace(strings.TrimRight(string(part.Part_name[:]), "\x00"))

			if partitionName == mountedName {
				partitionStart = part.Part_start
				found = true
				break
			}
		}
	}

	if !found {
		for i, part := range mbr.Mbr_partitions {
			if part.Part_status != '0' && part.Part_status != 0 {
				partitionName := strings.TrimSpace(strings.TrimRight(string(part.Part_name[:]), "\x00"))
				fmt.Printf("   %d: '%s' (inicio: %d, tamaño: %d)\n",
					i+1, partitionName, part.Part_start, part.Part_s)
			}
		}
		return
	}

	// Leer superbloque
	superblock, err := readSuperBlockMixed(file, partitionStart)
	if err != nil {
		fmt.Printf("❌ Error al leer el superbloque: %v\n", err)
		return
	}

	// Validar valores del superbloque
	if superblock.S_inodes_count <= 0 || superblock.S_inodes_count > 1000000 {

		// Intentar lectura estándar del superbloque
		file.Seek(partitionStart, 0)
		if err := binary.Read(file, binary.LittleEndian, &superblock); err != nil {
			fmt.Printf("❌ Error al leer superbloque con LittleEndian: %v\n", err)
			return
		}
	}

	htmlContent := generateInodeHTML(file, superblock, partition.Name)
	generateHTMLReport(htmlContent, outputPath, "INODE")
}

// generateBlockReport genera el reporte de bloques
func generateBlockReport(partition *MountedPartition, outputPath string) {
	file, err := os.Open(partition.Path)
	if err != nil {
		fmt.Printf("❌ Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	// Leer MBR para encontrar la partición
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("❌ Error al leer el MBR: %v\n", err)
		return
	}

	var partitionStart int64
	found := false

	// Normalizar el nombre de la partición montada
	mountedName := strings.TrimSpace(strings.TrimRight(partition.Name, "\x00"))

	for _, part := range mbr.Mbr_partitions {
		if part.Part_status != '0' && part.Part_status != 0 {
			partitionName := strings.TrimSpace(strings.TrimRight(string(part.Part_name[:]), "\x00"))

			if partitionName == mountedName {
				partitionStart = part.Part_start
				found = true
				break
			}
		}
	}

	if !found {
		fmt.Printf("❌ Error: No se encontró la partición '%s'\n", mountedName)
		return
	}

	// Leer superbloque
	superblock, err := readSuperBlockMixed(file, partitionStart)
	if err != nil {
		fmt.Printf("❌ Error al leer el superbloque: %v\n", err)
		return
	}

	htmlContent := generateBlockHTML(file, superblock, partition.Name)
	generateHTMLReport(htmlContent, outputPath, "BLOCK")
}

// generateBitmapInodeReport genera el reporte del bitmap de inodos
func generateBitmapInodeReport(partition *MountedPartition, outputPath string) {
	file, err := os.Open(partition.Path)
	if err != nil {
		fmt.Printf("❌ Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	// Leer MBR para encontrar la partición
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("❌ Error al leer el MBR: %v\n", err)
		return
	}

	var partitionStart int64
	found := false

	// Normalizar el nombre de la partición montada
	mountedName := strings.TrimSpace(strings.TrimRight(partition.Name, "\x00"))

	for _, part := range mbr.Mbr_partitions {
		if part.Part_status != '0' && part.Part_status != 0 {
			partitionName := strings.TrimSpace(strings.TrimRight(string(part.Part_name[:]), "\x00"))

			if partitionName == mountedName {
				partitionStart = part.Part_start
				found = true
				break
			}
		}
	}

	if !found {
		fmt.Printf("❌ Error: No se encontró la partición '%s'\n", mountedName)
		return
	}

	// Leer superbloque
	superblock, err := readSuperBlockMixed(file, partitionStart)
	if err != nil {
		fmt.Printf("❌ Error al leer el superbloque: %v\n", err)
		return
	}

	// Generar contenido del bitmap
	txtContent := generateBitmapInodeTxt(file, superblock, partition.Name)

	// Asegurar extensión .txt
	if !strings.HasSuffix(strings.ToLower(outputPath), ".txt") {
		outputPath = strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".txt"
	}

	// Escribir archivo de texto
	if err := os.WriteFile(outputPath, []byte(txtContent), 0644); err != nil {
		fmt.Printf("❌ Error al escribir archivo TXT: %v\n", err)
		return
	}

	fmt.Printf("✅ Reporte BITMAP_INODE generado: %s\n", outputPath)
}

// generateBitmapBlockReport genera el reporte del bitmap de bloques
func generateBitmapBlockReport(partition *MountedPartition, outputPath string) {
	file, err := os.Open(partition.Path)
	if err != nil {
		fmt.Printf("❌ Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	// Leer MBR para encontrar la partición
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("❌ Error al leer el MBR: %v\n", err)
		return
	}

	var partitionStart int64
	found := false

	// Normalizar el nombre de la partición montada
	mountedName := strings.TrimSpace(strings.TrimRight(partition.Name, "\x00"))

	for _, part := range mbr.Mbr_partitions {
		if part.Part_status != '0' && part.Part_status != 0 {
			partitionName := strings.TrimSpace(strings.TrimRight(string(part.Part_name[:]), "\x00"))

			if partitionName == mountedName {
				partitionStart = part.Part_start
				found = true
				break
			}
		}
	}

	if !found {
		fmt.Printf("❌ Error: No se encontró la partición '%s'\n", mountedName)
		return
	}

	// Leer superbloque
	superblock, err := readSuperBlockMixed(file, partitionStart)
	if err != nil {
		fmt.Printf("❌ Error al leer el superbloque: %v\n", err)
		return
	}

	// Generar contenido del bitmap de bloques
	txtContent := generateBitmapBlockTxt(file, superblock, partition.Name)

	// Asegurar extensión .txt
	if !strings.HasSuffix(strings.ToLower(outputPath), ".txt") {
		outputPath = strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".txt"
	}

	// Escribir archivo de texto
	if err := os.WriteFile(outputPath, []byte(txtContent), 0644); err != nil {
		fmt.Printf("❌ Error al escribir archivo TXT: %v\n", err)
		return
	}

	fmt.Printf("✅ Reporte BITMAP_BLOCK generado: %s\n", outputPath)
}

// generateTreeReport genera el reporte del árbol del sistema de archivos
func generateTreeReport(partition *MountedPartition, outputPath string) {
	file, err := os.Open(partition.Path)
	if err != nil {
		fmt.Printf("❌ Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	// Leer MBR para encontrar la partición
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("❌ Error al leer el MBR: %v\n", err)
		return
	}

	var partitionStart int64
	found := false

	// Normalizar el nombre de la partición montada
	mountedName := strings.TrimSpace(strings.TrimRight(partition.Name, "\x00"))

	for _, part := range mbr.Mbr_partitions {
		if part.Part_status != '0' && part.Part_status != 0 {
			partitionName := strings.TrimSpace(strings.TrimRight(string(part.Part_name[:]), "\x00"))

			if partitionName == mountedName {
				partitionStart = part.Part_start
				found = true
				break
			}
		}
	}

	if !found {
		fmt.Printf("❌ Error: No se encontró la partición '%s'\n", mountedName)
		return
	}

	// Leer superbloque
	superblock, err := readSuperBlockMixed(file, partitionStart)
	if err != nil {
		fmt.Printf("❌ Error al leer el superbloque: %v\n", err)
		return
	}

	htmlContent := generateTreeHTML(file, superblock, partition.Name)
	generateHTMLReport(htmlContent, outputPath, "TREE")
}

// generateFileReport genera el reporte de un archivo específico
func generateFileReport(partition *MountedPartition, outputPath string, filePath string) {
	if filePath == "" {
		fmt.Println("❌ Error: El parámetro -path_file_ls es obligatorio para el reporte 'file'")
		return
	}

	file, err := os.Open(partition.Path)
	if err != nil {
		fmt.Printf("❌ Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	// Leer MBR para encontrar la partición
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("❌ Error al leer el MBR: %v\n", err)
		return
	}

	var partitionStart int64
	found := false

	// Normalizar el nombre de la partición montada
	mountedName := strings.TrimSpace(strings.TrimRight(partition.Name, "\x00"))

	for _, part := range mbr.Mbr_partitions {
		if part.Part_status != '0' && part.Part_status != 0 {
			partitionName := strings.TrimSpace(strings.TrimRight(string(part.Part_name[:]), "\x00"))

			if partitionName == mountedName {
				partitionStart = part.Part_start
				found = true
				break
			}
		}
	}

	if !found {
		fmt.Printf("❌ Error: No se encontró la partición '%s'\n", mountedName)
		return
	}

	// Leer superbloque
	superblock, err := readSuperBlockMixed(file, partitionStart)
	if err != nil {
		fmt.Printf("❌ Error al leer el superbloque: %v\n", err)
		return
	}

	// Generar contenido del archivo
	txtContent := generateFileTxt(file, superblock, filePath, partition.Name)

	// Asegurar extensión .txt
	if !strings.HasSuffix(strings.ToLower(outputPath), ".txt") {
		outputPath = strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".txt"
	}

	// Escribir archivo de texto
	if err := os.WriteFile(outputPath, []byte(txtContent), 0644); err != nil {
		fmt.Printf("❌ Error al escribir archivo TXT: %v\n", err)
		return
	}

	fmt.Printf("✅ Reporte FILE generado: %s\n", outputPath)
}

// generateLsReport genera el reporte de listado de directorio
func generateLsReport(partition *MountedPartition, outputPath string, dirPath string) {
	if dirPath == "" {
		dirPath = "/" // Directorio raíz por defecto
	}

	file, err := os.Open(partition.Path)
	if err != nil {
		fmt.Printf("❌ Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	// Leer MBR para encontrar la partición
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("❌ Error al leer el MBR: %v\n", err)
		return
	}

	var partitionStart int64
	found := false

	// Normalizar el nombre de la partición montada
	mountedName := strings.TrimSpace(strings.TrimRight(partition.Name, "\x00"))

	for _, part := range mbr.Mbr_partitions {
		if part.Part_status != '0' && part.Part_status != 0 {
			partitionName := strings.TrimSpace(strings.TrimRight(string(part.Part_name[:]), "\x00"))

			if partitionName == mountedName {
				partitionStart = part.Part_start
				found = true
				break
			}
		}
	}

	if !found {
		fmt.Printf("❌ Error: No se encontró la partición '%s'\n", mountedName)
		return
	}

	// Leer superbloque
	superblock, err := readSuperBlockMixed(file, partitionStart)
	if err != nil {
		fmt.Printf("❌ Error al leer el superbloque: %v\n", err)
		return
	}

	htmlContent := generateLsHTML(file, superblock, dirPath, partition.Name)
	generateHTMLReport(htmlContent, outputPath, "LS")
}

// readEBRs lee todos los EBRs de una partición extendida
func readEBRs(diskPath string, extendedPartition structs.Partition) []structs.EBR {
	var ebrs []structs.EBR

	file, err := os.Open(diskPath)
	if err != nil {
		fmt.Printf("❌ Error al abrir disco para leer EBRs: %v\n", err)
		return ebrs
	}
	defer file.Close()

	currentPos := extendedPartition.Part_start

	for currentPos != -1 && currentPos != 0 {
		// Leer EBR en la posición actual
		file.Seek(currentPos, 0)

		var ebr structs.EBR
		if err := binary.Read(file, binary.LittleEndian, &ebr); err != nil {
			fmt.Printf("❌ Error al leer EBR en posición %d: %v\n", currentPos, err)
			break
		}

		// Si el EBR tiene una partición válida, agregarlo
		if ebr.PartMount != 0 {
			ebrs = append(ebrs, ebr)
		}

		// Avanzar al siguiente EBR
		currentPos = ebr.PartNext

		// Prevenir bucles infinitos
		if len(ebrs) > 10 {
			fmt.Println("⚠️  Demasiados EBRs, posible bucle infinito")
			break
		}
	}

	return ebrs
}

// generateHTMLReport genera el archivo HTML con estilos modernos
func generateHTMLReport(htmlContent string, outputPath string, reportType string) {
	// Asegurar extensión .html
	if !strings.HasSuffix(strings.ToLower(outputPath), ".html") {
		outputPath = strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".html"
	}

	// Escribir contenido HTML
	if err := os.WriteFile(outputPath, []byte(htmlContent), 0644); err != nil {
		fmt.Printf("❌ Error al escribir archivo HTML: %v\n", err)
		return
	}

	fmt.Printf("✅ Reporte %s generado: %s\n", reportType, outputPath)
	fmt.Printf("🌐 Abre en tu navegador: file://%s\n", outputPath)
}

// generateMBRHTML genera el reporte MBR en HTML moderno
func generateMBRHTML(mbr structs.MBR, diskPath string) string {
	var html strings.Builder

	html.WriteString(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Reporte MBR - ExtreamFS</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            border-radius: 20px;
            padding: 30px;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
        }
        
        .header {
            text-align: center;
            margin-bottom: 40px;
            padding-bottom: 20px;
            border-bottom: 3px solid #667eea;
        }
        
        .header h1 {
            color: #2c3e50;
            font-size: 2.5rem;
            margin-bottom: 10px;
            background: linear-gradient(45deg, #667eea, #764ba2);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .header .subtitle {
            color: #7f8c8d;
            font-size: 1.1rem;
            font-weight: 300;
        }
        
        .card {
            background: linear-gradient(135deg, #f8f9fa 0%, #e9ecef 100%);
            border-radius: 15px;
            padding: 25px;
            margin-bottom: 30px;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.1);
            border-left: 5px solid #667eea;
            transition: transform 0.3s ease, box-shadow 0.3s ease;
        }
        
        .card:hover {
            transform: translateY(-5px);
            box-shadow: 0 15px 40px rgba(0, 0, 0, 0.15);
        }
        
        .card-title {
            color: #2c3e50;
            font-size: 1.5rem;
            margin-bottom: 20px;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        
        .card-title .icon {
            font-size: 1.8rem;
        }
        
        .info-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 15px;
        }
        
        .info-item {
            display: flex;
            justify-content: space-between;
            padding: 12px 18px;
            background: rgba(255, 255, 255, 0.8);
            border-radius: 10px;
            border-left: 4px solid #3498db;
            transition: all 0.3s ease;
        }
        
        .info-item:hover {
            background: rgba(255, 255, 255, 1);
            transform: translateX(5px);
        }
        
        .info-label {
            font-weight: 600;
            color: #34495e;
        }
        
        .info-value {
            color: #2c3e50;
            font-family: 'Courier New', monospace;
            background: rgba(52, 152, 219, 0.1);
            padding: 4px 8px;
            border-radius: 5px;
        }
        
        .partition-primary {
            border-left-color: #27ae60;
        }
        
        .partition-primary .info-item {
            border-left-color: #27ae60;
        }
        
        .partition-extended {
            border-left-color: #f39c12;
        }
        
        .partition-extended .info-item {
            border-left-color: #f39c12;
        }
        
        .partition-logical {
            border-left-color: #e74c3c;
        }
        
        .partition-logical .info-item {
            border-left-color: #e74c3c;
        }
        
        .ebr-container {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(350px, 1fr));
            gap: 20px;
            margin-top: 20px;
        }
        
        .animation-fade {
            animation: fadeInUp 0.6s ease forwards;
        }
        
        @keyframes fadeInUp {
            from {
                opacity: 0;
                transform: translateY(30px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }
        
        .status-badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 0.85rem;
            font-weight: 600;
            text-transform: uppercase;
        }
        
        .status-active {
            background: linear-gradient(45deg, #27ae60, #2ecc71);
            color: white;
        }
        
        .status-inactive {
            background: linear-gradient(45deg, #95a5a6, #bdc3c7);
            color: white;
        }
        
        .footer {
            text-align: center;
            margin-top: 40px;
            padding-top: 20px;
            border-top: 2px solid #ecf0f1;
            color: #7f8c8d;
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 20px;
                margin: 10px;
            }
            
            .header h1 {
                font-size: 2rem;
            }
            
            .info-grid {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>📊 Reporte MBR</h1>
            <p class="subtitle">Master Boot Record - ExtreamFS</p>
        </div>`)

	// Información del MBR
	html.WriteString(`
        <div class="card animation-fade">
            <h2 class="card-title">
                <span class="icon">🔧</span>
                Información del Master Boot Record
            </h2>
            <div class="info-grid">`)

	// Mostrar tamaño del disco sin validación restrictiva
	mbrSize := mbr.Mbr_tamano
	var sizeDisplay string
	if mbrSize > 0 {
		sizeDisplay = fmt.Sprintf("%d bytes (%.2f MB)", mbrSize, float64(mbrSize)/(1024*1024))
	} else {
		sizeDisplay = fmt.Sprintf("%d bytes (valor inválido)", mbrSize)
	}

	html.WriteString(fmt.Sprintf(`
                    <div class="info-item">
                        <span class="info-label">Tamaño del Disco:</span>
                        <span class="info-value">%s</span>
                    </div>`, sizeDisplay))

	// Fecha de creación con validación más flexible
	var fechaDisplay string
	if mbr.Mbr_fecha_creacion > 0 && mbr.Mbr_fecha_creacion < 4102444800 { // Hasta año 2100
		fecha := time.Unix(mbr.Mbr_fecha_creacion, 0).Format("2006-01-02 15:04:05")
		fechaDisplay = fecha
	} else {
		fechaDisplay = fmt.Sprintf("No definida (valor: %d)", mbr.Mbr_fecha_creacion)
	}

	html.WriteString(fmt.Sprintf(`
                    <div class="info-item">
                        <span class="info-label">Fecha de Creación:</span>
                        <span class="info-value">%s</span>
                    </div>`, fechaDisplay))

	html.WriteString(fmt.Sprintf(`
                    <div class="info-item">
                        <span class="info-label">Signature del Disco:</span>
                        <span class="info-value">%d</span>
                    </div>`, mbr.Mbr_dsk_signature))

	// Algoritmo de ajuste
	fit := mbr.Dsk_fit
	if fit != 'F' && fit != 'B' && fit != 'W' && fit != 'f' && fit != 'b' && fit != 'w' {
		fit = '?'
	}
	fitText := map[byte]string{
		'F': "First Fit", 'f': "First Fit",
		'B': "Best Fit", 'b': "Best Fit",
		'W': "Worst Fit", 'w': "Worst Fit",
		'?': "No definido",
	}[fit]

	html.WriteString(fmt.Sprintf(`
                <div class="info-item">
                    <span class="info-label">Algoritmo de Ajuste:</span>
                    <span class="info-value">%s (%c)</span>
                </div>`, fitText, fit))

	html.WriteString(`
            </div>
        </div>`)

	// Particiones Primarias y Extendidas
	extendedPartition := -1
	partitionCount := 0

	for i, partition := range mbr.Mbr_partitions {
		if partition.Part_status != 0 && partition.Part_start > 0 && partition.Part_s > 0 {
			partitionCount++
			name := strings.TrimSpace(string(partition.Part_name[:]))
			if name == "" {
				name = fmt.Sprintf("Partición_%d", i+1)
			}

			partitionClass := "partition-primary"
			partitionType := "🟢 Primaria"

			if partition.Part_type == 'E' || partition.Part_type == 'e' {
				partitionClass = "partition-extended"
				partitionType = "🟡 Extendida"
				extendedPartition = i
			}

			statusText := "Activa"
			statusClass := "status-active"
			if partition.Part_status == '0' {
				statusText = "Inactiva"
				statusClass = "status-inactive"
			}

			fitPartText := map[byte]string{
				'F': "First Fit", 'f': "First Fit",
				'B': "Best Fit", 'b': "Best Fit",
				'W': "Worst Fit", 'w': "Worst Fit",
			}[partition.Part_fit]
			if fitPartText == "" {
				fitPartText = "No definido"
			}

			html.WriteString(fmt.Sprintf(`
        <div class="card %s animation-fade">
            <h2 class="card-title">
                <span class="icon">💾</span>
                %s - %s
            </h2>
            <div class="info-grid">
                <div class="info-item">
                    <span class="info-label">Estado:</span>
                    <span class="status-badge %s">%s</span>
                </div>
                <div class="info-item">
                    <span class="info-label">Tipo:</span>
                    <span class="info-value">%c</span>
                </div>
                <div class="info-item">
                    <span class="info-label">Algoritmo de Ajuste:</span>
                    <span class="info-value">%s</span>
                </div>
                <div class="info-item">
                    <span class="info-label">Posición de Inicio:</span>
                    <span class="info-value">%d bytes</span>
                </div>
                <div class="info-item">
                    <span class="info-label">Tamaño:</span>
                    <span class="info-value">%d bytes</span>
                </div>
                <div class="info-item">
                    <span class="info-label">Nombre:</span>
                    <span class="info-value">%s</span>
                </div>
            </div>
        </div>`, partitionClass, partitionType, name, statusClass, statusText,
				partition.Part_type, fitPartText, partition.Part_start, partition.Part_s, name))
		}
	}

	// EBRs si hay partición extendida
	if extendedPartition >= 0 {
		ebrs := readEBRs(diskPath, mbr.Mbr_partitions[extendedPartition])

		if len(ebrs) > 0 {
			html.WriteString(`
        <div class="card partition-logical animation-fade">
            <h2 class="card-title">
                <span class="icon">🔗</span>
                Particiones Lógicas (EBRs)
            </h2>
            <div class="ebr-container">`)

			for i, ebr := range ebrs {
				name := strings.TrimSpace(string(ebr.PartName[:]))
				if name == "" {
					name = fmt.Sprintf("Lógica_%d", i+1)
				}

				statusText := "Activa"
				statusClass := "status-active"
				if ebr.PartMount == 0 {
					statusText = "Inactiva"
					statusClass = "status-inactive"
				}

				fitText := map[byte]string{
					'F': "First Fit", 'f': "First Fit",
					'B': "Best Fit", 'b': "Best Fit",
					'W': "Worst Fit", 'w': "Worst Fit",
				}[ebr.PartFit]
				if fitText == "" {
					fitText = "No definido"
				}

				html.WriteString(fmt.Sprintf(`
                <div class="card" style="margin-bottom: 0;">
                    <h3 style="color: #e74c3c; margin-bottom: 15px;">🔴 EBR %d - %s</h3>
                    <div class="info-grid">
                        <div class="info-item">
                            <span class="info-label">Estado:</span>
                            <span class="status-badge %s">%s</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Algoritmo:</span>
                            <span class="info-value">%s</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Inicio:</span>
                            <span class="info-value">%d bytes</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Tamaño:</span>
                            <span class="info-value">%d bytes</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Siguiente EBR:</span>
                            <span class="info-value">%d</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Nombre:</span>
                            <span class="info-value">%s</span>
                        </div>
                    </div>
                </div>`, i+1, name, statusClass, statusText, fitText,
					ebr.PartStart, ebr.PartS, ebr.PartNext, name))
			}

			html.WriteString(`
            </div>
        </div>`)
		}
	}

	// Footer
	html.WriteString(fmt.Sprintf(`
        <div class="footer">
            <p>📊 Reporte generado por <strong>ExtreamFS </strong></p>
            <p>🕒 %s | 💿 %s</p>
            <p>📁 Total de particiones encontradas: <strong>%d</strong></p>
        </div>
    </div>
    
    <script>
        // Animaciones adicionales
        const cards = document.querySelectorAll('.card');
        cards.forEach((card, index) => {
            card.style.animationDelay = (index * 0.1) + 's';
        });
        
        // Efectos hover mejorados
        document.addEventListener('DOMContentLoaded', function() {
            const infoItems = document.querySelectorAll('.info-item');
            infoItems.forEach(item => {
                item.addEventListener('mouseenter', function() {
                    this.style.transform = 'translateX(5px) scale(1.02)';
                });
                item.addEventListener('mouseleave', function() {
                    this.style.transform = 'translateX(0) scale(1)';
                });
            });
        });
    </script>
</body>
</html>`, time.Now().Format("2006-01-02 15:04:05"), diskPath, partitionCount))

	return html.String()
}

func generateDiskHTML(mbr structs.MBR, diskPath string, fileInfo os.FileInfo) string {
	var html strings.Builder

	html.WriteString(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Reporte DISK - ExtreamFS</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 1400px;
            margin: 0 auto;
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            border-radius: 20px;
            padding: 30px;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
        }
        
        .header {
            text-align: center;
            margin-bottom: 40px;
            padding-bottom: 20px;
            border-bottom: 3px solid #667eea;
        }
        
        .header h1 {
            color: #2c3e50;
            font-size: 2.5rem;
            margin-bottom: 10px;
            background: linear-gradient(45deg, #667eea, #764ba2);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .header .subtitle {
            color: #7f8c8d;
            font-size: 1.1rem;
            font-weight: 300;
        }
        
        .disk-container {
            background: white;
            border-radius: 15px;
            padding: 30px;
            margin-bottom: 30px;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.1);
        }
        
        .disk-title {
            text-align: center;
            font-size: 1.8rem;
            color: #2c3e50;
            margin-bottom: 30px;
            font-weight: 600;
        }
        
        .disk-visual {
            display: flex;
            width: 100%;
            height: 120px;
            border: 3px solid #34495e;
            border-radius: 10px;
            overflow: hidden;
            margin-bottom: 30px;
            box-shadow: 0 5px 15px rgba(0, 0, 0, 0.1);
        }
        
        .disk-segment {
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            padding: 10px;
            color: white;
            font-weight: 600;
            text-align: center;
            font-size: 0.9rem;
            position: relative;
            transition: all 0.3s ease;
            cursor: pointer;
        }
        
        .disk-segment:hover {
            transform: scaleY(1.05);
            z-index: 10;
            box-shadow: 0 5px 20px rgba(0, 0, 0, 0.3);
        }
        
        .segment-mbr {
            background: linear-gradient(135deg, #e74c3c, #c0392b);
        }
        
        .segment-primary {
            background: linear-gradient(135deg, #27ae60, #229954);
        }
        
        .segment-extended {
            background: linear-gradient(135deg, #f39c12, #e67e22);
        }
        
        .segment-logical {
            background: linear-gradient(135deg, #3498db, #2980b9);
        }
        
        .segment-free {
            background: linear-gradient(135deg, #95a5a6, #7f8c8d);
        }
        
        .segment-label {
            font-size: 0.8rem;
            font-weight: 700;
            margin-bottom: 5px;
        }
        
        .segment-size {
            font-size: 0.7rem;
            opacity: 0.9;
        }
        
        .legend {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin-bottom: 30px;
        }
        
        .legend-item {
            display: flex;
            align-items: center;
            padding: 12px 15px;
            background: rgba(255, 255, 255, 0.8);
            border-radius: 10px;
            border-left: 5px solid;
            transition: all 0.3s ease;
        }
        
        .legend-item:hover {
            background: rgba(255, 255, 255, 1);
            transform: translateX(5px);
        }
        
        .legend-color {
            width: 20px;
            height: 20px;
            border-radius: 50%;
            margin-right: 12px;
            border: 2px solid rgba(255, 255, 255, 0.3);
        }
        
        .legend-mbr { border-left-color: #e74c3c; }
        .legend-mbr .legend-color { background: linear-gradient(135deg, #e74c3c, #c0392b); }
        
        .legend-primary { border-left-color: #27ae60; }
        .legend-primary .legend-color { background: linear-gradient(135deg, #27ae60, #229954); }
        
        .legend-extended { border-left-color: #f39c12; }
        .legend-extended .legend-color { background: linear-gradient(135deg, #f39c12, #e67e22); }
        
        .legend-logical { border-left-color: #3498db; }
        .legend-logical .legend-color { background: linear-gradient(135deg, #3498db, #2980b9); }
        
        .legend-free { border-left-color: #95a5a6; }
        .legend-free .legend-color { background: linear-gradient(135deg, #95a5a6, #7f8c8d); }
        
        .legend-text {
            flex: 1;
        }
        
        .legend-label {
            font-weight: 600;
            color: #2c3e50;
            margin-bottom: 2px;
        }
        
        .legend-details {
            font-size: 0.85rem;
            color: #7f8c8d;
        }
        
        .details-table {
            width: 100%;
            border-collapse: collapse;
            background: white;
            border-radius: 10px;
            overflow: hidden;
            box-shadow: 0 5px 15px rgba(0, 0, 0, 0.1);
        }
        
        .details-table th {
            background: linear-gradient(135deg, #667eea, #764ba2);
            color: white;
            padding: 15px;
            text-align: left;
            font-weight: 600;
        }
        
        .details-table td {
            padding: 12px 15px;
            border-bottom: 1px solid #ecf0f1;
            transition: background-color 0.3s ease;
        }
        
        .details-table tr:hover td {
            background: #f8f9fa;
        }
        
        .details-table tr:nth-child(even) td {
            background: rgba(102, 126, 234, 0.05);
        }
        
        .footer {
            text-align: center;
            margin-top: 40px;
            padding-top: 20px;
            border-top: 2px solid #ecf0f1;
            color: #7f8c8d;
        }
        
        .animation-slide {
            animation: slideInFromLeft 0.8s ease forwards;
        }
        
        @keyframes slideInFromLeft {
            from {
                opacity: 0;
                transform: translateX(-100px);
            }
            to {
                opacity: 1;
                transform: translateX(0);
            }
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 20px;
                margin: 10px;
            }
            
            .disk-visual {
                height: 100px;
            }
            
            .legend {
                grid-template-columns: 1fr;
            }
            
            .segment-label {
                font-size: 0.7rem;
            }
            
            .segment-size {
                font-size: 0.6rem;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>💽 Reporte DISK</h1>
            <p class="subtitle">Estructura del Disco - ExtreamFS </p>
        </div>`)

	// Calcular estructura del disco
	diskStructure := calculateDiskStructure(mbr, fileInfo.Size())

	// Mostrar visualización del disco
	html.WriteString(`
        <div class="disk-container animation-slide">
            <h2 class="disk-title">📊 ` + filepath.Base(diskPath) + `</h2>
            <div class="disk-visual">`)

	// Generar segmentos visuales
	for _, segment := range diskStructure {
		html.WriteString(fmt.Sprintf(`
                <div class="disk-segment %s" style="flex: %f;" title="%s">
                    <div class="segment-label">%s</div>
                    <div class="segment-size">%.1f%%</div>
                </div>`,
			segment.CSSClass, segment.Percentage/100, segment.Tooltip,
			segment.Label, segment.Percentage))
	}

	html.WriteString(`
            </div>`)

	// Leyenda
	html.WriteString(`
            <div class="legend">`)

	for _, segment := range diskStructure {
		html.WriteString(fmt.Sprintf(`
                <div class="legend-item %s">
                    <div class="legend-color"></div>
                    <div class="legend-text">
                        <div class="legend-label">%s</div>
                        <div class="legend-details">%s - %.1f%%</div>
                    </div>
                </div>`,
			segment.LegendClass, segment.Label, segment.Details, segment.Percentage))
	}

	html.WriteString(`
            </div>`)

	// Tabla de detalles
	html.WriteString(`
            <table class="details-table">
                <thead>
                    <tr>
                        <th>Tipo</th>
                        <th>Nombre</th>
                        <th>Inicio (bytes)</th>
                        <th>Tamaño (bytes)</th>
                        <th>Porcentaje</th>
                        <th>Estado</th>
                    </tr>
                </thead>
                <tbody>`)

	// Llenar tabla con detalles
	for _, segment := range diskStructure {
		html.WriteString(fmt.Sprintf(`
                    <tr>
                        <td><strong>%s</strong></td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td><strong>%.1f%%</strong></td>
                        <td>%s</td>
                    </tr>`,
			segment.Type, segment.Name, segment.StartStr, segment.SizeStr,
			segment.Percentage, segment.Status))
	}

	html.WriteString(`
                </tbody>
            </table>
        </div>`)

	// Footer
	totalPercentage := 0.0
	for _, segment := range diskStructure {
		totalPercentage += segment.Percentage
	}

	html.WriteString(fmt.Sprintf(`
        <div class="footer">
            <p>💽 Reporte generado por <strong>ExtreamFS </strong></p>
            <p>🕒 %s | 📁 %s</p>
            <p>📊 Total verificado: <strong>%.1f%%</strong> | 💾 Tamaño total: <strong>%s</strong></p>
        </div>
    </div>
    
    <script>
        // Animaciones dinámicas para los segmentos
        document.addEventListener('DOMContentLoaded', function() {
            const segments = document.querySelectorAll('.disk-segment');
            segments.forEach((segment, index) => {
                segment.style.animationDelay = (index * 0.2) + 's';
                segment.style.animation = 'slideInFromLeft 0.8s ease forwards';
            });
            
            // Efectos de hover mejorados
            segments.forEach(segment => {
                segment.addEventListener('mouseenter', function() {
                    this.style.transform = 'scaleY(1.1) translateY(-5px)';
                    this.style.zIndex = '20';
                });
                segment.addEventListener('mouseleave', function() {
                    this.style.transform = 'scaleY(1) translateY(0)';
                    this.style.zIndex = '1';
                });
            });
        });
    </script>
</body>
</html>`, time.Now().Format("2006-01-02 15:04:05"), diskPath, totalPercentage,
		formatBytes(fileInfo.Size())))

	return html.String()
}

func generateInodeHTML(file *os.File, superblock structs.SuperBloque, partitionName string) string {
	var html strings.Builder

	html.WriteString(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Reporte INODE - ExtreamFS</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 1600px;
            margin: 0 auto;
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            border-radius: 20px;
            padding: 30px;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
        }
        
        .header {
            text-align: center;
            margin-bottom: 40px;
            padding-bottom: 20px;
            border-bottom: 3px solid #667eea;
        }
        
        .header h1 {
            color: #2c3e50;
            font-size: 2.5rem;
            margin-bottom: 10px;
            background: linear-gradient(45deg, #667eea, #764ba2);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .header .subtitle {
            color: #7f8c8d;
            font-size: 1.1rem;
            font-weight: 300;
        }
        
        .inode-chain {
            display: flex;
            flex-wrap: wrap;
            justify-content: center;
            align-items: flex-start;
            gap: 30px;
            margin: 30px 0;
            padding: 20px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 15px;
            min-height: 400px;
        }
        
        .inode-block {
            background: linear-gradient(135deg, #ffffff, #f8f9fa);
            border: 3px solid #34495e;
            border-radius: 12px;
            padding: 20px;
            width: 280px;
            box-shadow: 0 8px 25px rgba(0, 0, 0, 0.15);
            transition: all 0.3s ease;
            position: relative;
            animation: fadeInScale 0.6s ease forwards;
        }
        
        .inode-block:hover {
            transform: translateY(-8px) scale(1.02);
            box-shadow: 0 15px 35px rgba(0, 0, 0, 0.2);
            border-color: #3498db;
        }
        
        .inode-header {
            background: linear-gradient(135deg, #3498db, #2980b9);
            color: white;
            text-align: center;
            padding: 12px;
            margin: -20px -20px 15px -20px;
            border-radius: 9px 9px 0 0;
            font-weight: 700;
            font-size: 1.1rem;
        }
        
        .inode-field {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 8px 0;
            border-bottom: 1px solid #ecf0f1;
            transition: background-color 0.3s ease;
        }
        
        .inode-field:hover {
            background: rgba(52, 152, 219, 0.05);
            border-radius: 5px;
            padding-left: 8px;
            padding-right: 8px;
        }
        
        .field-label {
            font-weight: 600;
            color: #34495e;
            font-size: 0.9rem;
        }
        
        .field-value {
            color: #2c3e50;
            font-family: 'Courier New', monospace;
            background: rgba(52, 152, 219, 0.1);
            padding: 3px 8px;
            border-radius: 4px;
            font-size: 0.9rem;
        }
        
        .arrow {
            font-size: 2.5rem;
            color: #3498db;
            align-self: center;
            animation: pulse 2s infinite;
            filter: drop-shadow(0 0 10px rgba(52, 152, 219, 0.3));
        }
        
        .inode-used {
            border-color: #27ae60;
        }
        
        .inode-used .inode-header {
            background: linear-gradient(135deg, #27ae60, #229954);
        }
        
        .inode-directory {
            border-color: #f39c12;
        }
        
        .inode-directory .inode-header {
            background: linear-gradient(135deg, #f39c12, #e67e22);
        }
        
        .inode-file {
            border-color: #e74c3c;
        }
        
        .inode-file .inode-header {
            background: linear-gradient(135deg, #e74c3c, #c0392b);
        }
        
        .blocks-container {
            margin-top: 10px;
            padding-top: 10px;
            border-top: 2px solid #ecf0f1;
        }
        
        .block-item {
            display: inline-block;
            background: linear-gradient(135deg, #9b59b6, #8e44ad);
            color: white;
            padding: 4px 8px;
            margin: 2px;
            border-radius: 15px;
            font-size: 0.8rem;
            font-weight: 600;
        }
        
        .block-unused {
            background: linear-gradient(135deg, #95a5a6, #7f8c8d);
        }
        
        .timestamp {
            color: #7f8c8d;
            font-size: 0.8rem;
            font-style: italic;
        }
        
        .permission-badge {
            background: linear-gradient(135deg, #16a085, #1abc9c);
            color: white;
            padding: 2px 6px;
            border-radius: 10px;
            font-size: 0.8rem;
            font-weight: 600;
        }
        
        .empty-state {
            text-align: center;
            padding: 60px 20px;
            color: #7f8c8d;
        }
        
        .empty-icon {
            font-size: 4rem;
            margin-bottom: 20px;
        }
        
        .footer {
            text-align: center;
            margin-top: 40px;
            padding-top: 20px;
            border-top: 2px solid #ecf0f1;
            color: #7f8c8d;
        }
        
        @keyframes fadeInScale {
            from {
                opacity: 0;
                transform: scale(0.8) translateY(30px);
            }
            to {
                opacity: 1;
                transform: scale(1) translateY(0);
            }
        }
        
        @keyframes pulse {
            0%, 100% { transform: scale(1); }
            50% { transform: scale(1.1); }
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 20px;
                margin: 10px;
            }
            
            .inode-chain {
                flex-direction: column;
                align-items: center;
            }
            
            .inode-block {
                width: 100%;
                max-width: 300px;
            }
            
            .arrow {
                transform: rotate(90deg);
                margin: 10px 0;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>📁 Reporte INODE</h1>
            <p class="subtitle">Lista Enlazada de Inodos - ExtreamFS </p>
        </div>`)

	// Leer inodos desde el superbloque
	inodes := readInodesFromPartition(file, superblock)
	usedInodes := filterUsedInodes(inodes)

	// Mostrar solo la lista enlazada de inodos
	if len(usedInodes) > 0 {
		html.WriteString(`
        <div class="inode-chain">`)

		for i, inode := range usedInodes {
			// Determinar el tipo de inodo
			inodeClass := "inode-used"
			iconType := "📄"
			typeText := "Archivo"

			// Interpretar el tipo correctamente
			var realType int64
			if inode.I_type >= 48 && inode.I_type <= 57 { // ASCII '0'-'9'
				realType = int64(inode.I_type - 48) // Convertir ASCII a número
			} else {
				realType = int64(inode.I_type)
			}

			// Detectar tipos
			switch realType {
			case 0: // Directorio
				inodeClass = "inode-directory"
				iconType = "📁"
				typeText = "Directorio"
			case 1: // Archivo regular
				inodeClass = "inode-file"
				iconType = "📄"
				typeText = "Archivo"
			default:
				// Detectar por tamaño
				if inode.I_s == 96 { // Tamaño típico de directorio
					inodeClass = "inode-directory"
					iconType = "📁"
					typeText = fmt.Sprintf("Directorio (tipo %d)", realType)
				} else if inode.I_s > 0 && inode.I_s < 96 {
					inodeClass = "inode-file"
					iconType = "📄"
					typeText = fmt.Sprintf("Archivo (tipo %d)", realType)
				} else {
					inodeClass = "inode-used"
					iconType = "❓"
					typeText = fmt.Sprintf("Desconocido (tipo %d)", realType)
				}
			}

			html.WriteString(fmt.Sprintf(`
            <div class="inode-block %s" style="animation-delay: %dms;">
                <div class="inode-header">
                    %s Inodo %d
                </div>`, inodeClass, i*200, iconType, i))

			// Campos del inodo
			html.WriteString(fmt.Sprintf(`
                <div class="inode-field">
                    <span class="field-label">I_uid:</span>
                    <span class="field-value">%d</span>
                </div>
                <div class="inode-field">
                    <span class="field-label">I_size:</span>
                    <span class="field-value">%d bytes</span>
                </div>
                <div class="inode-field">
                    <span class="field-label">I_atime:</span>
                    <span class="field-value timestamp">%s</span>
                </div>
                <div class="inode-field">
                    <span class="field-label">I_ctime:</span>
                    <span class="field-value timestamp">%s</span>
                </div>
                <div class="inode-field">
                    <span class="field-label">I_mtime:</span>
                    <span class="field-value timestamp">%s</span>
                </div>
                <div class="inode-field">
                    <span class="field-label">I_type:</span>
                    <span class="field-value">%s</span>
                </div>
                <div class="inode-field">
                    <span class="field-label">I_perm:</span>
                    <span class="permission-badge">%s</span>
                </div>`,
				inode.I_uid, inode.I_s,
				formatTimestamp(inode.I_atime),
				formatTimestamp(inode.I_ctime),
				formatTimestamp(inode.I_mtime),
				typeText, formatPermissions(inode.I_perm)))

			// Mostrar bloques asignados
			html.WriteString(`
                <div class="blocks-container">
                    <div class="field-label" style="margin-bottom: 8px;">Bloques asignados:</div>`)

			validBlocks := 0
			for j := 0; j < 15; j++ {
				blockValue := inode.I_block[j]
				if blockValue != -1 && blockValue >= 0 {
					validBlocks++
					html.WriteString(fmt.Sprintf(`
                        <span class="block-item" title="Bloque %d: Posición %d">%d</span>`, j, blockValue, blockValue))
				}
			}

			if validBlocks == 0 {
				html.WriteString(`
                    <span class="block-item block-unused">Sin bloques asignados</span>`)
			} else {
				html.WriteString(fmt.Sprintf(`
                    <div style="margin-top: 8px; font-size: 0.8rem; color: #7f8c8d;">
                        Total: %d bloques asignados
                    </div>`, validBlocks))
			}

			html.WriteString(`
                </div>
            </div>`)

			// Agregar flecha si no es el último
			if i < len(usedInodes)-1 {
				html.WriteString(`
            <div class="arrow">→</div>`)
			}
		}

		html.WriteString(`
        </div>`)
	} else {
		// Estado vacío
		html.WriteString(`
        <div class="empty-state">
            <div class="empty-icon">📂</div>
            <h3>No hay inodos utilizados</h3>
            <p>La partición no contiene inodos en uso actualmente.</p>
        </div>`)
	}

	// Footer simplificado
	html.WriteString(fmt.Sprintf(`
        <div class="footer">
            <p>📁 Reporte generado por <strong>ExtreamFS </strong></p>
            <p>🕒 %s | 📦 Partición: <strong>%s</strong></p>
            <p>📊 Inodos mostrados: <strong>%d</strong></p>
        </div>
    </div>
    
    <script>
        // Efectos de animación
        document.addEventListener('DOMContentLoaded', function() {
            const inodeBlocks = document.querySelectorAll('.inode-block');
            
            inodeBlocks.forEach((block, index) => {
                block.addEventListener('mouseenter', function() {
                    this.style.transform = 'translateY(-12px) scale(1.03)';
                });
                
                block.addEventListener('mouseleave', function() {
                    this.style.transform = 'translateY(0) scale(1)';
                });
            });
            
            // Efecto de conexión entre inodos
            const arrows = document.querySelectorAll('.arrow');
            arrows.forEach((arrow, index) => {
                setTimeout(() => {
                    arrow.style.opacity = '1';
                    arrow.style.transform = 'scale(1)';
                }, index * 300 + 800);
            });
        });
    </script>
</body>
</html>`, time.Now().Format("2006-01-02 15:04:05"), partitionName, len(usedInodes)))

	return html.String()
}

func formatPermissions(perm [3]byte) string {
	if perm[0] == 0 && perm[1] == 0 && perm[2] == 0 {
		return "000"
	}

	// Si son valores ASCII, convertir a octales
	if perm[0] >= 48 && perm[0] <= 55 { // Rango ASCII de '0' a '7'
		return fmt.Sprintf("%c%c%c", perm[0], perm[1], perm[2])
	}

	// Si son valores numéricos directos
	return fmt.Sprintf("%d%d%d", perm[0], perm[1], perm[2])
}

// readInodesFromPartition lee todos los inodos de una partición
func readInodesFromPartition(file *os.File, superblock structs.SuperBloque) []structs.Inodos {
	var inodes []structs.Inodos

	// Posicionarse en el inicio de los inodos
	file.Seek(superblock.S_inode_start, 0)

	// Leer todos los inodos
	for i := int64(0); i < superblock.S_inodes_count; i++ {
		var inode structs.Inodos
		if err := binary.Read(file, binary.LittleEndian, &inode); err != nil {
			break
		}
		inodes = append(inodes, inode)
	}

	return inodes
}

// filterUsedInodes filtra solo los inodos que están en uso
func filterUsedInodes(inodes []structs.Inodos) []structs.Inodos {
	var usedInodes []structs.Inodos

	for _, inode := range inodes {
		// Usar la nueva función de validación más robusta
		if isInodeValid(inode) {
			// Agregar información de índice para debugging
			usedInodes = append(usedInodes, inode)
		}
	}

	return usedInodes
}

// formatTimestamp convierte timestamp a formato legible
func formatTimestamp(timestamp int64) string {
	if timestamp <= 0 {
		return "No definido"
	}

	// Validar que el timestamp esté en un rango razonable
	// Entre 1970 y 2100 (timestamps típicos de Unix)
	if timestamp < 0 || timestamp > 4102444800 {
		return fmt.Sprintf("Timestamp inválido (%d)", timestamp)
	}

	return time.Unix(timestamp, 0).Format("02/01/2006 15:04")
}
func isInodeValid(inode structs.Inodos) bool {
	// Verificar si tiene al menos un bloque válido
	hasValidBlock := false
	for _, block := range inode.I_block {
		if block != -1 && block >= 0 {
			hasValidBlock = true
			break
		}
	}

	// Es válido si tiene bloques asignados O tiene tamaño > 0
	return hasValidBlock && (inode.I_s > 0 || inode.I_uid >= 0)
}

func generateBlockHTML(file *os.File, superblock structs.SuperBloque, partitionName string) string {
	var html strings.Builder

	html.WriteString(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Reporte BLOCK - ExtreamFS</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 1600px;
            margin: 0 auto;
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            border-radius: 20px;
            padding: 30px;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
        }
        
        .header {
            text-align: center;
            margin-bottom: 40px;
            padding-bottom: 20px;
            border-bottom: 3px solid #667eea;
        }
        
        .header h1 {
            color: #2c3e50;
            font-size: 2.5rem;
            margin-bottom: 10px;
            background: linear-gradient(45deg, #667eea, #764ba2);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .header .subtitle {
            color: #7f8c8d;
            font-size: 1.1rem;
            font-weight: 300;
        }
        
        .block-chain {
            display: flex;
            flex-wrap: wrap;
            justify-content: center;
            align-items: flex-start;
            gap: 30px;
            margin: 30px 0;
            padding: 20px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 15px;
            min-height: 400px;
        }
        
        .block-item {
            background: linear-gradient(135deg, #ffffff, #f8f9fa);
            border: 3px solid #34495e;
            border-radius: 12px;
            padding: 20px;
            width: 350px;
            box-shadow: 0 8px 25px rgba(0, 0, 0, 0.15);
            transition: all 0.3s ease;
            position: relative;
            animation: fadeInScale 0.6s ease forwards;
        }
        
        .block-item:hover {
            transform: translateY(-8px) scale(1.02);
            box-shadow: 0 15px 35px rgba(0, 0, 0, 0.2);
            border-color: #3498db;
        }
        
        .block-header {
            background: linear-gradient(135deg, #3498db, #2980b9);
            color: white;
            text-align: center;
            padding: 12px;
            margin: -20px -20px 15px -20px;
            border-radius: 9px 9px 0 0;
            font-weight: 700;
            font-size: 1.1rem;
        }
        
        .block-content {
            font-family: 'Courier New', monospace;
            background: rgba(52, 152, 219, 0.1);
            padding: 15px;
            border-radius: 8px;
            max-height: 300px;
            overflow-y: auto;
            font-size: 0.85rem;
            line-height: 1.4;
            white-space: pre-wrap;
            word-break: break-all;
        }
        
        .block-folder {
            border-color: #f39c12;
        }
        
        .block-folder .block-header {
            background: linear-gradient(135deg, #f39c12, #e67e22);
        }
        
        .block-file {
            border-color: #e74c3c;
        }
        
        .block-file .block-header {
            background: linear-gradient(135deg, #e74c3c, #c0392b);
        }
        
        .block-pointer {
            border-color: #9b59b6;
        }
        
        .block-pointer .block-header {
            background: linear-gradient(135deg, #9b59b6, #8e44ad);
        }
        
        .arrow {
            font-size: 2.5rem;
            color: #3498db;
            align-self: center;
            animation: pulse 2s infinite;
            filter: drop-shadow(0 0 10px rgba(52, 152, 219, 0.3));
        }
        
        .folder-entry {
            display: flex;
            justify-content: space-between;
            padding: 2px 0;
            border-bottom: 1px solid rgba(52, 152, 219, 0.2);
        }
        
        .folder-entry:last-child {
            border-bottom: none;
        }
        
        .entry-name {
            font-weight: 600;
            color: #2c3e50;
        }
        
        .entry-inode {
            color: #7f8c8d;
            font-size: 0.8rem;
        }
        
        .empty-state {
            text-align: center;
            padding: 60px 20px;
            color: #7f8c8d;
        }
        
        .empty-icon {
            font-size: 4rem;
            margin-bottom: 20px;
        }
        
        .footer {
            text-align: center;
            margin-top: 40px;
            padding-top: 20px;
            border-top: 2px solid #ecf0f1;
            color: #7f8c8d;
        }
        
        @keyframes fadeInScale {
            from {
                opacity: 0;
                transform: scale(0.8) translateY(30px);
            }
            to {
                opacity: 1;
                transform: scale(1) translateY(0);
            }
        }
        
        @keyframes pulse {
            0%, 100% { transform: scale(1); }
            50% { transform: scale(1.1); }
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 20px;
                margin: 10px;
            }
            
            .block-chain {
                flex-direction: column;
                align-items: center;
            }
            
            .block-item {
                width: 100%;
                max-width: 350px;
            }
            
            .arrow {
                transform: rotate(90deg);
                margin: 10px 0;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🧱 Reporte BLOCK</h1>
            <p class="subtitle">Lista Enlazada de Bloques - ExtreamFS </p>
        </div>`)

	// Leer bloques utilizados
	usedBlocks := readUsedBlocksFromPartition(file, superblock)

	// Mostrar solo la lista enlazada de bloques
	if len(usedBlocks) > 0 {
		html.WriteString(`
        <div class="block-chain">`)

		for i, blockData := range usedBlocks {
			// Determinar el tipo de bloque
			blockClass := "block-item"
			iconType := "📄"
			blockType := "Archivo"

			// Detectar tipo por contenido
			switch blockData.Type {
			case "folder":
				blockClass = "block-folder"
				iconType = "📁"
				blockType = "Carpeta"
			case "file":
				blockClass = "block-file"
				iconType = "📄"
				blockType = "Archivo"
			case "pointer":
				blockClass = "block-pointer"
				iconType = "🔗"
				blockType = "Apuntadores"
			}

			html.WriteString(fmt.Sprintf(`
            <div class="%s" style="animation-delay: %dms;">
                <div class="block-header">
                    %s Bloque %s %d
                </div>`, blockClass, i*200, iconType, blockType, blockData.Index))

			// Contenido del bloque
			html.WriteString(fmt.Sprintf(`
                <div class="block-content">%s</div>
            </div>`, blockData.Content))

			// Agregar flecha si no es el último
			if i < len(usedBlocks)-1 {
				html.WriteString(`
            <div class="arrow">→</div>`)
			}
		}

		html.WriteString(`
        </div>`)
	} else {
		// Estado vacío
		html.WriteString(`
        <div class="empty-state">
            <div class="empty-icon">🧱</div>
            <h3>No hay bloques utilizados</h3>
            <p>La partición no contiene bloques en uso actualmente.</p>
        </div>`)
	}

	// Footer simplificado
	html.WriteString(fmt.Sprintf(`
        <div class="footer">
            <p>🧱 Reporte generado por <strong>ExtreamFS </strong></p>
            <p>🕒 %s | 📦 Partición: <strong>%s</strong></p>
            <p>📊 Bloques mostrados: <strong>%d</strong></p>
        </div>
    </div>
    
    <script>
        // Efectos de animación
        document.addEventListener('DOMContentLoaded', function() {
            const blockItems = document.querySelectorAll('.block-item');
            
            blockItems.forEach((block, index) => {
                block.addEventListener('mouseenter', function() {
                    this.style.transform = 'translateY(-12px) scale(1.03)';
                });
                
                block.addEventListener('mouseleave', function() {
                    this.style.transform = 'translateY(0) scale(1)';
                });
            });
            
            // Efecto de conexión entre bloques
            const arrows = document.querySelectorAll('.arrow');
            arrows.forEach((arrow, index) => {
                setTimeout(() => {
                    arrow.style.opacity = '1';
                    arrow.style.transform = 'scale(1)';
                }, index * 300 + 800);
            });
        });
    </script>
</body>
</html>`, time.Now().Format("2006-01-02 15:04:05"), partitionName, len(usedBlocks)))

	return html.String()
}

// Estructura para almacenar información de bloques
type BlockData struct {
	Index   int64
	Type    string
	Content string
}

// readUsedBlocksFromPartition lee todos los bloques utilizados de una partición
func readUsedBlocksFromPartition(file *os.File, superblock structs.SuperBloque) []BlockData {
	var usedBlocks []BlockData

	// Primero obtener los inodos utilizados para saber qué bloques están en uso
	inodes := readInodesFromPartition(file, superblock)
	usedInodes := filterUsedInodes(inodes)

	// Set para evitar bloques duplicados
	seenBlocks := make(map[int64]bool)

	// Para cada inodo utilizado, leer sus bloques
	for _, inode := range usedInodes {
		for _, blockIndex := range inode.I_block {
			if blockIndex != -1 && blockIndex >= 0 && !seenBlocks[blockIndex] {
				seenBlocks[blockIndex] = true

				// Leer el contenido del bloque
				blockPosition := superblock.S_block_start + (blockIndex * superblock.S_block_s)
				file.Seek(blockPosition, 0)

				// Determinar el tipo de bloque según el tipo de inodo
				var blockType string
				var realType int64
				if inode.I_type >= 48 && inode.I_type <= 57 {
					realType = int64(inode.I_type - 48)
				} else {
					realType = int64(inode.I_type)
				}

				if realType == 0 || inode.I_s == 96 {
					blockType = "folder"
				} else {
					blockType = "file"
				}

				content := readBlockContent(file, superblock, blockIndex, blockType)

				usedBlocks = append(usedBlocks, BlockData{
					Index:   blockIndex,
					Type:    blockType,
					Content: content,
				})
			}
		}
	}

	return usedBlocks
}

// readBlockContent lee y formatea el contenido de un bloque
func readBlockContent(file *os.File, superblock structs.SuperBloque, blockIndex int64, blockType string) string {
	blockPosition := superblock.S_block_start + (blockIndex * superblock.S_block_s)
	file.Seek(blockPosition, 0)

	switch blockType {
	case "folder":
		// Leer como bloque de carpeta
		var folderBlock structs.BloqueCarpeta
		if err := binary.Read(file, binary.LittleEndian, &folderBlock); err != nil {
			return fmt.Sprintf("Error al leer bloque de carpeta: %v", err)
		}

		var content strings.Builder

		// Formatear el contenido como en el ejemplo
		for i, content_entry := range folderBlock.BContent {
			name := strings.TrimRight(string(content_entry.BName[:]), "\x00")
			if name != "" {
				content.WriteString(fmt.Sprintf("%-12s %d\n", name, content_entry.BInodo))
			} else if i < 4 { // Mostrar entradas vacías solo para las primeras 4
				content.WriteString(fmt.Sprintf("%-12s %d\n", "", content_entry.BInodo))
			}
		}

		return content.String()

	case "file":
		// Leer como bloque de archivo
		var fileBlock structs.BloqueArchivo
		if err := binary.Read(file, binary.LittleEndian, &fileBlock); err != nil {
			return fmt.Sprintf("Error al leer bloque de archivo: %v", err)
		}

		// Convertir contenido del archivo a string
		content := strings.TrimRight(string(fileBlock.BContent[:]), "\x00")
		if content == "" {
			return "(archivo vacío)"
		}

		return content

	case "pointer":
		// Leer como bloque de apuntadores
		var pointerBlock structs.BloqueApuntador
		if err := binary.Read(file, binary.LittleEndian, &pointerBlock); err != nil {
			return fmt.Sprintf("Error al leer bloque de apuntadores: %v", err)
		}

		var content strings.Builder
		for i, pointer := range pointerBlock.BPointers {
			if i%5 == 0 && i > 0 {
				content.WriteString(",\n")
			} else if i > 0 {
				content.WriteString(", ")
			}
			content.WriteString(fmt.Sprintf("%d", pointer))
		}

		return content.String()
	}

	// Leer contenido raw si no se puede determinar el tipo
	buffer := make([]byte, superblock.S_block_s)
	file.Read(buffer)

	// Convertir a string, mostrando solo caracteres imprimibles
	var content strings.Builder
	for i, b := range buffer {
		if b >= 32 && b <= 126 { // Caracteres imprimibles ASCII
			content.WriteByte(b)
		} else if b == 0 {
			if i < 100 { // Solo mostrar algunos ceros al principio
				content.WriteString("\\0")
			} else {
				break // Dejar de mostrar después de muchos ceros
			}
		} else {
			content.WriteString(fmt.Sprintf("\\x%02x", b))
		}

		// Limitar la longitud del contenido mostrado
		if content.Len() > 500 {
			content.WriteString("...")
			break
		}
	}

	return content.String()
}

func generateBitmapInodeTxt(file *os.File, superblock structs.SuperBloque, partitionName string) string {
	var content strings.Builder

	// Encabezado del reporte
	content.WriteString("==================================================\n")
	content.WriteString("           REPORTE BITMAP DE INODOS\n")
	content.WriteString("              ExtreamFS \n")
	content.WriteString("==================================================\n")
	content.WriteString(fmt.Sprintf("Partición: %s\n", partitionName))
	content.WriteString(fmt.Sprintf("Fecha: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	content.WriteString(fmt.Sprintf("Total de inodos: %d\n", superblock.S_inodes_count))
	content.WriteString(fmt.Sprintf("Inodos libres: %d\n", superblock.S_free_inodes_count))
	content.WriteString(fmt.Sprintf("Inodos usados: %d\n", superblock.S_inodes_count-superblock.S_free_inodes_count))
	content.WriteString("==================================================\n")
	content.WriteString("Bitmap (0=Libre, 1=Usado) - 20 registros por línea:\n")
	content.WriteString("==================================================\n\n")

	// Leer el bitmap de inodos
	bitmapData := readInodeBitmap(file, superblock)

	// Convertir bitmap a bits individuales
	bits := convertBitmapToBits(bitmapData, int(superblock.S_inodes_count))

	// Mostrar bits en formato de 20 por línea con numeración
	lineNumber := 0
	for i := 0; i < len(bits); i += 20 {
		// Número de línea (empezando desde 0)
		content.WriteString(fmt.Sprintf("%2d: ", lineNumber))

		// Mostrar hasta 20 bits en esta línea
		end := i + 20
		if end > len(bits) {
			end = len(bits)
		}

		for j := i; j < end; j++ {
			content.WriteString(fmt.Sprintf("%d ", bits[j]))
		}

		// Información adicional de la línea
		usedInLine := 0
		for j := i; j < end; j++ {
			if bits[j] == 1 {
				usedInLine++
			}
		}

		content.WriteString(fmt.Sprintf("  [Inodos %d-%d: %d usados, %d libres]",
			i, end-1, usedInLine, (end-i)-usedInLine))

		content.WriteString("\n")
		lineNumber++
	}

	// Estadísticas finales
	content.WriteString("\n==================================================\n")
	content.WriteString("                  ESTADÍSTICAS\n")
	content.WriteString("==================================================\n")

	totalUsed := 0
	totalFree := 0
	for _, bit := range bits {
		if bit == 1 {
			totalUsed++
		} else {
			totalFree++
		}
	}

	content.WriteString(fmt.Sprintf("Total de inodos analizados: %d\n", len(bits)))
	content.WriteString(fmt.Sprintf("Inodos marcados como usados: %d\n", totalUsed))
	content.WriteString(fmt.Sprintf("Inodos marcados como libres: %d\n", totalFree))
	content.WriteString(fmt.Sprintf("Porcentaje de uso: %.2f%%\n", float64(totalUsed)/float64(len(bits))*100))
	content.WriteString(fmt.Sprintf("Posición del bitmap: %d bytes\n", superblock.S_bm_inode_start))
	content.WriteString(fmt.Sprintf("Tamaño del bitmap: %d bytes\n", (superblock.S_inodes_count+7)/8)) // Redondear hacia arriba

	// Verificación de consistencia
	if int64(totalFree) != superblock.S_free_inodes_count {
		content.WriteString("\n⚠️  ADVERTENCIA: Inconsistencia detectada!\n")
		content.WriteString(fmt.Sprintf("   Superbloque dice %d libres, bitmap muestra %d libres\n",
			superblock.S_free_inodes_count, totalFree))
	}

	content.WriteString("\n==================================================\n")
	content.WriteString("           Fin del reporte bitmap inodos\n")
	content.WriteString("==================================================\n")

	return content.String()
}

// readInodeBitmap lee el bitmap de inodos desde el disco
func readInodeBitmap(file *os.File, superblock structs.SuperBloque) []byte {
	// Calcular el tamaño del bitmap en bytes
	bitmapSizeBytes := (superblock.S_inodes_count + 7) / 8 // Redondear hacia arriba

	// Posicionarse en el inicio del bitmap de inodos
	file.Seek(superblock.S_bm_inode_start, 0)

	// Leer el bitmap completo
	bitmapData := make([]byte, bitmapSizeBytes)
	_, err := file.Read(bitmapData)
	if err != nil {
		fmt.Printf("⚠️  Error al leer bitmap de inodos: %v\n", err)
		return bitmapData
	}

	return bitmapData
}

// convertBitmapToBits convierte el bitmap de bytes a bits individuales
func convertBitmapToBits(bitmapData []byte, totalInodes int) []int {
	var bits []int

	bitCount := 0
	for _, byteValue := range bitmapData {
		// Procesar cada bit del byte (del bit 0 al 7)
		for bitPos := 0; bitPos < 8 && bitCount < totalInodes; bitPos++ {
			// Extraer el bit en la posición bitPos
			bit := (byteValue >> bitPos) & 1
			bits = append(bits, int(bit))
			bitCount++
		}

		// Si ya hemos procesado todos los inodos, salir
		if bitCount >= totalInodes {
			break
		}
	}

	return bits
}

func generateBitmapBlockTxt(file *os.File, superblock structs.SuperBloque, partitionName string) string {
	var content strings.Builder

	// Encabezado del reporte
	content.WriteString("==================================================\n")
	content.WriteString("           REPORTE BITMAP DE BLOQUES\n")
	content.WriteString("              ExtreamFS \n")
	content.WriteString("==================================================\n")
	content.WriteString(fmt.Sprintf("Partición: %s\n", partitionName))
	content.WriteString(fmt.Sprintf("Fecha: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	content.WriteString(fmt.Sprintf("Total de bloques: %d\n", superblock.S_blocks_count))
	content.WriteString(fmt.Sprintf("Bloques libres: %d\n", superblock.S_free_blocks_count))
	content.WriteString(fmt.Sprintf("Bloques usados: %d\n", superblock.S_blocks_count-superblock.S_free_blocks_count))
	content.WriteString("==================================================\n")
	content.WriteString("Bitmap (0=Libre, 1=Usado) - 20 registros por línea:\n")
	content.WriteString("==================================================\n\n")

	// Leer el bitmap de bloques
	bitmapData := readBlockBitmap(file, superblock)

	// Convertir bitmap a bits individuales
	bits := convertBitmapToBits(bitmapData, int(superblock.S_blocks_count))

	// Mostrar bits en formato de 20 por línea con numeración
	lineNumber := 0
	for i := 0; i < len(bits); i += 20 {
		// Número de línea
		content.WriteString(fmt.Sprintf("%2d: ", lineNumber))

		// Mostrar hasta 20 bits
		end := i + 20
		if end > len(bits) {
			end = len(bits)
		}

		for j := i; j < end; j++ {
			content.WriteString(fmt.Sprintf("%d ", bits[j]))
		}

		// Información adicional
		usedInLine := 0
		for j := i; j < end; j++ {
			if bits[j] == 1 {
				usedInLine++
			}
		}

		content.WriteString(fmt.Sprintf("  [Bloques %d-%d: %d usados, %d libres]",
			i, end-1, usedInLine, (end-i)-usedInLine))

		content.WriteString("\n")
		lineNumber++
	}

	// Estadísticas finales
	content.WriteString("\n==================================================\n")
	content.WriteString("                  ESTADÍSTICAS\n")
	content.WriteString("==================================================\n")

	totalUsed := 0
	totalFree := 0
	for _, bit := range bits {
		if bit == 1 {
			totalUsed++
		} else {
			totalFree++
		}
	}

	content.WriteString(fmt.Sprintf("Total de bloques analizados: %d\n", len(bits)))
	content.WriteString(fmt.Sprintf("Bloques marcados como usados: %d\n", totalUsed))
	content.WriteString(fmt.Sprintf("Bloques marcados como libres: %d\n", totalFree))
	content.WriteString(fmt.Sprintf("Porcentaje de uso: %.2f%%\n", float64(totalUsed)/float64(len(bits))*100))
	content.WriteString(fmt.Sprintf("Posición del bitmap: %d bytes\n", superblock.S_bm_block_start))
	content.WriteString(fmt.Sprintf("Tamaño del bitmap: %d bytes\n", (superblock.S_blocks_count+7)/8))

	content.WriteString("\n==================================================\n")
	content.WriteString("           Fin del reporte bitmap bloques\n")
	content.WriteString("==================================================\n")

	return content.String()
}

// readBlockBitmap lee el bitmap de bloques desde el disco
func readBlockBitmap(file *os.File, superblock structs.SuperBloque) []byte {
	// Calcular el tamaño del bitmap en bytes
	bitmapSizeBytes := (superblock.S_blocks_count + 7) / 8 // Redondear hacia arriba

	// Posicionarse en el inicio del bitmap de bloques
	file.Seek(superblock.S_bm_block_start, 0)

	// Leer el bitmap completo
	bitmapData := make([]byte, bitmapSizeBytes)
	_, err := file.Read(bitmapData)
	if err != nil {
		fmt.Printf("⚠️  Error al leer bitmap de bloques: %v\n", err)
		return bitmapData
	}

	return bitmapData
}

// Estructura para representar nodos del árbol
type TreeNode struct {
	Type      string // "inode", "folder_block", "file_block", "pointer_block"
	Index     int64  // Índice del inodo o bloque
	Name      string // Nombre del archivo/directorio
	Content   string // Contenido formateado
	Children  []*TreeNode
	InodeData *structs.Inodos
	BlockData interface{} // BloqueCarpeta, BloqueArchivo, o BloqueApuntador
	Level     int         // Nivel en el árbol para el layout
	X, Y      int         // Coordenadas para el layout
}

// generateTreeHTML genera el reporte del árbol en HTML
func generateTreeHTML(file *os.File, superblock structs.SuperBloque, partitionName string) string {
	var html strings.Builder

	html.WriteString(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Reporte TREE - ExtreamFS</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 100%;
            margin: 0 auto;
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            border-radius: 20px;
            padding: 30px;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
        }
        
        .header {
            text-align: center;
            margin-bottom: 40px;
            padding-bottom: 20px;
            border-bottom: 3px solid #667eea;
        }
        
        .header h1 {
            color: #2c3e50;
            font-size: 2.5rem;
            margin-bottom: 10px;
            background: linear-gradient(45deg, #667eea, #764ba2);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .header .subtitle {
            color: #7f8c8d;
            font-size: 1.1rem;
            font-weight: 300;
        }
        
        .tree-wrapper {
            background: rgba(255, 255, 255, 0.1);
            border-radius: 15px;
            padding: 20px;
            margin-bottom: 30px;
            border: 2px solid rgba(102, 126, 234, 0.2);
        }
        
        .tree-container {
            position: relative;
            width: 100%;
            height: 800px;
            background: linear-gradient(135deg, #f8f9fa 0%, #e9ecef 100%);
            border-radius: 10px;
            padding: 20px;
            overflow: auto;
            border: 1px solid #dee2e6;
            box-shadow: inset 0 2px 10px rgba(0, 0, 0, 0.1);
        }
        
        .tree-canvas {
            position: relative;
            min-width: 2000px;
            min-height: 1500px;
            width: max-content;
            height: max-content;
        }
        
        .tree-node {
            position: absolute;
            background: linear-gradient(135deg, #ffffff, #f8f9fa);
            border: 2px solid #34495e;
            border-radius: 8px;
            padding: 12px;
            min-width: 140px;
            max-width: 200px;
            box-shadow: 0 4px 15px rgba(0, 0, 0, 0.15);
            transition: all 0.3s ease;
            font-size: 0.85rem;
            z-index: 5;
        }
        
        .tree-node:hover {
            transform: scale(1.05);
            box-shadow: 0 8px 25px rgba(0, 0, 0, 0.25);
            z-index: 15;
        }
        
        .inode-node {
            background: linear-gradient(135deg, #3498db, #2980b9);
            color: white;
            border-color: #2980b9;
        }
        
        .folder-block-node {
            background: linear-gradient(135deg, #f39c12, #e67e22);
            color: white;
            border-color: #e67e22;
        }
        
        .file-block-node {
            background: linear-gradient(135deg, #e74c3c, #c0392b);
            color: white;
            border-color: #c0392b;
        }
        
        .pointer-block-node {
            background: linear-gradient(135deg, #9b59b6, #8e44ad);
            color: white;
            border-color: #8e44ad;
        }
        
        .node-header {
            font-weight: bold;
            margin-bottom: 8px;
            font-size: 0.9rem;
            text-align: center;
            padding-bottom: 5px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.3);
        }
        
        .node-content {
            font-size: 0.75rem;
            line-height: 1.3;
            white-space: pre-line;
            max-height: 120px;
            overflow-y: auto;
            font-family: 'Courier New', monospace;
        }
        
        .connection-line {
            position: absolute;
            background: linear-gradient(45deg, #3498db, #2980b9);
            z-index: 1;
            border-radius: 1px;
        }
        
        .connection-horizontal {
            height: 3px;
        }
        
        .connection-vertical {
            width: 3px;
        }
        
        .arrow {
            position: absolute;
            width: 0;
            height: 0;
            border-left: 8px solid transparent;
            border-right: 8px solid transparent;
            border-top: 10px solid #3498db;
            z-index: 3;
            filter: drop-shadow(0 2px 4px rgba(0, 0, 0, 0.2));
        }
        
        .legend {
            display: flex;
            justify-content: center;
            gap: 20px;
            margin-bottom: 20px;
            flex-wrap: wrap;
        }
        
        .legend-item {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 10px 15px;
            background: rgba(255, 255, 255, 0.9);
            border-radius: 25px;
            font-size: 0.9rem;
            font-weight: 600;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
            transition: all 0.3s ease;
        }
        
        .legend-item:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 15px rgba(0, 0, 0, 0.15);
        }
        
        .legend-color {
            width: 18px;
            height: 18px;
            border-radius: 4px;
            border: 2px solid rgba(255, 255, 255, 0.8);
        }
        
        .legend-inode { background: linear-gradient(135deg, #3498db, #2980b9); }
        .legend-folder { background: linear-gradient(135deg, #f39c12, #e67e22); }
        .legend-file { background: linear-gradient(135deg, #e74c3c, #c0392b); }
        .legend-pointer { background: linear-gradient(135deg, #9b59b6, #8e44ad); }
        
        .scroll-hint {
            text-align: center;
            color: #7f8c8d;
            font-size: 0.9rem;
            margin-bottom: 15px;
            padding: 10px;
            background: rgba(127, 140, 141, 0.1);
            border-radius: 20px;
            border: 1px dashed #7f8c8d;
        }
        
        .empty-state {
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            height: 100%;
            color: #7f8c8d;
            text-align: center;
        }
        
        .empty-icon {
            font-size: 4rem;
            margin-bottom: 20px;
            opacity: 0.7;
        }
        
        .footer {
            text-align: center;
            margin-top: 30px;
            padding-top: 20px;
            border-top: 2px solid #ecf0f1;
            color: #7f8c8d;
            background: rgba(255, 255, 255, 0.5);
            border-radius: 10px;
            padding: 20px;
        }
        
        .tree-controls {
            display: flex;
            justify-content: center;
            gap: 10px;
            margin-bottom: 15px;
        }
        
        .control-btn {
            padding: 8px 16px;
            background: linear-gradient(135deg, #667eea, #764ba2);
            color: white;
            border: none;
            border-radius: 20px;
            font-size: 0.8rem;
            cursor: pointer;
            transition: all 0.3s ease;
        }
        
        .control-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 15px rgba(102, 126, 234, 0.3);
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 15px;
                margin: 10px;
            }
            
            .tree-container {
                height: 600px;
            }
            
            .tree-node {
                min-width: 120px;
                max-width: 160px;
                font-size: 0.8rem;
                padding: 10px;
            }
            
            .legend {
                gap: 10px;
            }
            
            .legend-item {
                padding: 8px 12px;
                font-size: 0.85rem;
            }
        }
        
        @keyframes fadeInScale {
            from {
                opacity: 0;
                transform: scale(0.8);
            }
            to {
                opacity: 1;
                transform: scale(1);
            }
        }
        
        .tree-node {
            animation: fadeInScale 0.6s ease forwards;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🌳 Reporte TREE</h1>
            <p class="subtitle">Árbol del Sistema de Archivos - ExtreamFS </p>
        </div>
        
        <div class="legend">
            <div class="legend-item">
                <div class="legend-color legend-inode"></div>
                <span>Inodos</span>
            </div>
            <div class="legend-item">
                <div class="legend-color legend-folder"></div>
                <span>Bloques Carpeta</span>
            </div>
            <div class="legend-item">
                <div class="legend-color legend-file"></div>
                <span>Bloques Archivo</span>
            </div>
            <div class="legend-item">
                <div class="legend-color legend-pointer"></div>
                <span>Bloques Apuntadores</span>
            </div>
        </div>`)

	// Construir el árbol del sistema de archivos
	tree := buildFileSystemTree(file, superblock)

	if tree != nil {
		html.WriteString(`
        <div class="tree-wrapper">
            <div class="scroll-hint">
                💡 Usa las barras de desplazamiento para navegar por el árbol completo
            </div>
            
            <div class="tree-controls">
                <button class="control-btn" onclick="resetTreeView()">🔄 Centrar Vista</button>
                <button class="control-btn" onclick="zoomTree(1.2)">🔍 Zoom +</button>
                <button class="control-btn" onclick="zoomTree(0.8)">🔍 Zoom -</button>
            </div>
            
            <div class="tree-container" id="treeContainer">
                <div class="tree-canvas" id="treeCanvas">`)

		// Calcular posiciones para el layout mejorado
		layoutTreeImproved(tree, 50, 50)

		// Renderizar nodos y conexiones
		renderTreeNodes(tree, &html)
		renderTreeConnections(tree, &html)

		html.WriteString(`
                </div>
            </div>
        </div>`)
	} else {
		html.WriteString(`
        <div class="tree-wrapper">
            <div class="tree-container">
                <div class="empty-state">
                    <div class="empty-icon">🌳</div>
                    <h3>Sistema de archivos vacío</h3>
                    <p>No se encontraron inodos válidos para generar el árbol.</p>
                </div>
            </div>
        </div>`)
	}

	// Footer
	html.WriteString(fmt.Sprintf(`
        <div class="footer">
            <p>🌳 Reporte generado por <strong>ExtreamFS </strong></p>
            <p>🕒 %s | 📦 Partición: <strong>%s</strong></p>
            <p>📊 El árbol se muestra en un contenedor con scroll para mejor navegación</p>
        </div>
    </div>
    
    <script>
        let currentZoom = 1;
        
        // Animaciones para los nodos del árbol
        document.addEventListener('DOMContentLoaded', function() {
            const nodes = document.querySelectorAll('.tree-node');
            nodes.forEach((node, index) => {
                node.style.animationDelay = (index * 50) + 'ms';
            });
            
            // Centrar la vista inicial
            setTimeout(resetTreeView, 500);
        });
        
        function resetTreeView() {
            const container = document.getElementById('treeContainer');
            if (container) {
                container.scrollLeft = 0;
                container.scrollTop = 0;
            }
        }
        
        function zoomTree(factor) {
            currentZoom *= factor;
            
            // Limitar zoom entre 0.5 y 2.0
            currentZoom = Math.max(0.5, Math.min(2.0, currentZoom));
            
            const canvas = document.getElementById('treeCanvas');
            if (canvas) {
                canvas.style.transform = 'scale(' + currentZoom + ')';
                canvas.style.transformOrigin = 'top left';
                
                // Ajustar el tamaño del contenedor según el zoom
                const baseWidth = 2000;
                const baseHeight = 1500;
                canvas.style.width = (baseWidth * currentZoom) + 'px';
                canvas.style.height = (baseHeight * currentZoom) + 'px';
            }
        }
        
        // Mejorar la navegación con teclado
        document.addEventListener('keydown', function(e) {
            const container = document.getElementById('treeContainer');
            if (!container) return;
            
            const scrollSpeed = 50;
            
            switch(e.key) {
                case 'ArrowUp':
                    container.scrollTop -= scrollSpeed;
                    e.preventDefault();
                    break;
                case 'ArrowDown':
                    container.scrollTop += scrollSpeed;
                    e.preventDefault();
                    break;
                case 'ArrowLeft':
                    container.scrollLeft -= scrollSpeed;
                    e.preventDefault();
                    break;
                case 'ArrowRight':
                    container.scrollLeft += scrollSpeed;
                    e.preventDefault();
                    break;
                case 'Home':
                    resetTreeView();
                    e.preventDefault();
                    break;
            }
        });
        
        // Agregar indicadores de scroll
        const container = document.getElementById('treeContainer');
        if (container) {
            container.addEventListener('scroll', function() {
                const scrollPercentX = (this.scrollLeft / (this.scrollWidth - this.clientWidth)) * 100;
                const scrollPercentY = (this.scrollTop / (this.scrollHeight - this.clientHeight)) * 100;
                
                // Opcional: mostrar indicadores de posición
                console.log('Scroll: X=' + scrollPercentX.toFixed(1) + '%%, Y=' + scrollPercentY.toFixed(1) + '%%');
            });
        }
    </script>
</body>
</html>`, time.Now().Format("2006-01-02 15:04:05"), partitionName))

	return html.String()
}

func layoutTreeImproved(node *TreeNode, x, y int) {
	if node == nil {
		return
	}

	node.X = x
	node.Y = y

	// Calcular el ancho total necesario para todos los hijos
	totalWidth := 0
	if len(node.Children) > 0 {
		totalWidth = len(node.Children) * 220 // Espaciado horizontal mejorado
	}

	// Centrar los hijos
	startX := x - totalWidth/2
	if startX < 50 {
		startX = 50 // Margen mínimo
	}

	childY := y + 180 // Espaciado vertical mejorado

	for i, child := range node.Children {
		childX := startX + (i * 220)
		layoutTreeImproved(child, childX, childY)

		// Si es un bloque de carpeta con inodos hijos, dar más espacio
		if child.Type == "folder_block" && len(child.Children) > 0 {
			// Los hijos de este bloque se distribuirán automáticamente
			extraWidth := len(child.Children) * 60
			startX += extraWidth
		}
	}
}

// buildFileSystemTree construye el árbol del sistema de archivos
func buildFileSystemTree(file *os.File, superblock structs.SuperBloque) *TreeNode {
	// Leer todos los inodos
	inodes := readInodesFromPartition(file, superblock)
	usedInodes := filterUsedInodes(inodes)

	if len(usedInodes) == 0 {
		return nil
	}

	// Buscar el inodo raíz (generalmente el inodo 0 que es un directorio)
	var rootInode *structs.Inodos
	var rootIndex int

	for i, inode := range usedInodes {
		var realType int64
		if inode.I_type >= 48 && inode.I_type <= 57 {
			realType = int64(inode.I_type - 48)
		} else {
			realType = int64(inode.I_type)
		}

		// Buscar directorio que pueda ser raíz
		if realType == 0 || inode.I_s == 96 {
			rootInode = &inode
			rootIndex = i
			break
		}
	}

	if rootInode == nil {
		// Si no hay directorio, usar el primer inodo
		rootInode = &usedInodes[0]
		rootIndex = 0
	}

	// Crear nodo raíz
	rootNode := &TreeNode{
		Type:      "inode",
		Index:     int64(rootIndex),
		Name:      "Root",
		InodeData: rootInode,
		Level:     0,
		Children:  []*TreeNode{},
	}

	rootNode.Content = formatInodeContent(*rootInode, rootIndex)

	// Construir recursivamente el árbol
	buildInodeTree(file, superblock, rootNode, usedInodes, 1)

	return rootNode
}

// buildInodeTree construye recursivamente el árbol desde un inodo
func buildInodeTree(file *os.File, superblock structs.SuperBloque, parentNode *TreeNode, allInodes []structs.Inodos, level int) {
	if parentNode.InodeData == nil || level > 5 { // Límite de profundidad
		return
	}

	inode := *parentNode.InodeData

	// Procesar bloques del inodo
	for i := 0; i < 15; i++ {
		blockIndex := inode.I_block[i]
		if blockIndex == -1 || blockIndex < 0 {
			continue
		}

		// Determinar tipo de bloque
		var blockType string
		var realType int64
		if inode.I_type >= 48 && inode.I_type <= 57 {
			realType = int64(inode.I_type - 48)
		} else {
			realType = int64(inode.I_type)
		}

		if i < 12 { // Bloques directos
			if realType == 0 || inode.I_s == 96 {
				blockType = "folder_block"
			} else {
				blockType = "file_block"
			}
		} else { // Bloques de apuntadores
			blockType = "pointer_block"
		}

		// Crear nodo para el bloque
		blockNode := &TreeNode{
			Type:     blockType,
			Index:    blockIndex,
			Name:     fmt.Sprintf("Block_%d", blockIndex),
			Level:    level,
			Children: []*TreeNode{},
		}

		// Leer contenido del bloque
		switch blockType {
		case "folder_block":
			blockNode.Content, blockNode.BlockData = readFolderBlockForTree(file, superblock, blockIndex)

			// Si es bloque de carpeta, buscar inodos referenciados
			if folderBlock, ok := blockNode.BlockData.(structs.BloqueCarpeta); ok {
				for _, entry := range folderBlock.BContent {
					entryName := strings.TrimRight(string(entry.BName[:]), "\x00")
					if entryName != "" && entryName != "." && entryName != ".." && entry.BInodo >= 0 {
						// Buscar el inodo referenciado
						if int(entry.BInodo) < len(allInodes) {
							referencedInode := allInodes[entry.BInodo]

							childInodeNode := &TreeNode{
								Type:      "inode",
								Index:     int64(entry.BInodo),
								Name:      entryName,
								InodeData: &referencedInode,
								Level:     level + 1,
								Children:  []*TreeNode{},
							}

							childInodeNode.Content = formatInodeContent(referencedInode, int(entry.BInodo))

							// Recursión para construir subárbol
							buildInodeTree(file, superblock, childInodeNode, allInodes, level+2)

							blockNode.Children = append(blockNode.Children, childInodeNode)
						}
					}
				}
			}

		case "file_block":
			blockNode.Content = readFileBlockForTree(file, superblock, blockIndex)
		case "pointer_block":
			blockNode.Content = readPointerBlockForTree(file, superblock, blockIndex)
		}

		parentNode.Children = append(parentNode.Children, blockNode)
	}
}

// Funciones para leer contenido de bloques específicos para el árbol
func readFolderBlockForTree(file *os.File, superblock structs.SuperBloque, blockIndex int64) (string, interface{}) {
	blockPosition := superblock.S_block_start + (blockIndex * superblock.S_block_s)
	file.Seek(blockPosition, 0)

	var folderBlock structs.BloqueCarpeta
	if err := binary.Read(file, binary.LittleEndian, &folderBlock); err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}

	var content strings.Builder
	content.WriteString("b_name | b_inodo\n")
	content.WriteString("-------|--------\n")

	for _, entry := range folderBlock.BContent {
		name := strings.TrimRight(string(entry.BName[:]), "\x00")
		if name != "" || entry.BInodo != -1 {
			content.WriteString(fmt.Sprintf("%-6s | %d\n", name, entry.BInodo))
		}
	}

	return content.String(), folderBlock
}

func readFileBlockForTree(file *os.File, superblock structs.SuperBloque, blockIndex int64) string {
	blockPosition := superblock.S_block_start + (blockIndex * superblock.S_block_s)
	file.Seek(blockPosition, 0)

	var fileBlock structs.BloqueArchivo
	if err := binary.Read(file, binary.LittleEndian, &fileBlock); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	content := strings.TrimRight(string(fileBlock.BContent[:]), "\x00")
	if content == "" {
		return "(vacío)"
	}

	// Limitar longitud para visualización
	if len(content) > 50 {
		return content[:47] + "..."
	}

	return content
}

func readPointerBlockForTree(file *os.File, superblock structs.SuperBloque, blockIndex int64) string {
	blockPosition := superblock.S_block_start + (blockIndex * superblock.S_block_s)
	file.Seek(blockPosition, 0)

	var pointerBlock structs.BloqueApuntador
	if err := binary.Read(file, binary.LittleEndian, &pointerBlock); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	var content strings.Builder
	content.WriteString("Apuntadores:\n")
	validPointers := 0

	for i, pointer := range pointerBlock.BPointers {
		if pointer != -1 && pointer >= 0 {
			content.WriteString(fmt.Sprintf("[%d]: %d\n", i, pointer))
			validPointers++
			if validPointers >= 6 { // Limitar para visualización
				content.WriteString("...")
				break
			}
		}
	}

	return content.String()
}

func formatInodeContent(inode structs.Inodos, inodeIndex int) string {
	var content strings.Builder

	content.WriteString(fmt.Sprintf("Inodo %d\n", inodeIndex))
	content.WriteString(fmt.Sprintf("Size: %d\n", inode.I_s))
	content.WriteString(fmt.Sprintf("UID: %d\n", inode.I_uid))

	var realType int64
	if inode.I_type >= 48 && inode.I_type <= 57 {
		realType = int64(inode.I_type - 48)
	} else {
		realType = int64(inode.I_type)
	}

	if realType == 0 || inode.I_s == 96 {
		content.WriteString("Tipo: Dir")
	} else {
		content.WriteString("Tipo: File")
	}

	return content.String()
}

// renderTreeNodes renderiza todos los nodos del árbol
func renderTreeNodes(node *TreeNode, html *strings.Builder) {
	if node == nil {
		return
	}

	// Determinar clase CSS según el tipo
	nodeClass := ""
	switch node.Type {
	case "inode":
		nodeClass = "inode-node"
	case "folder_block":
		nodeClass = "folder-block-node"
	case "file_block":
		nodeClass = "file-block-node"
	case "pointer_block":
		nodeClass = "pointer-block-node"
	}

	// Renderizar nodo actual
	html.WriteString(fmt.Sprintf(
		`<div class="tree-node %s" style="left: %dpx; top: %dpx;">`+
			`<div class="node-header">%s</div>`+
			`<div class="node-content">%s</div>`+
			`</div>`,
		nodeClass, node.X, node.Y, node.Name, node.Content))

	// Renderizar nodos hijos recursivamente
	for _, child := range node.Children {
		renderTreeNodes(child, html)
	}
}

// renderTreeConnections renderiza las líneas de conexión entre nodos
func renderTreeConnections(node *TreeNode, html *strings.Builder) {
	if node == nil {
		return
	}

	// Renderizar conexiones a hijos
	for _, child := range node.Children {
		// Calcular posiciones de conexión
		startX := node.X + 90 // Centro del nodo padre
		startY := node.Y + 60 // Parte inferior del nodo padre
		endX := child.X + 90  // Centro del nodo hijo
		endY := child.Y       // Parte superior del nodo hijo

		// Línea vertical desde el padre
		html.WriteString(fmt.Sprintf(`
            <div class="connection-line connection-vertical" 
                 style="left: %dpx; top: %dpx; height: %dpx;"></div>`,
			startX, startY, 20))

		// Línea horizontal
		lineLeft := min(startX, endX)
		lineWidth := abs(endX - startX)
		if lineWidth > 0 {
			html.WriteString(fmt.Sprintf(`
                <div class="connection-line connection-horizontal" 
                     style="left: %dpx; top: %dpx; width: %dpx;"></div>`,
				lineLeft, startY+20, lineWidth))
		}

		// Línea vertical hacia el hijo
		html.WriteString(fmt.Sprintf(`
            <div class="connection-line connection-vertical" 
                 style="left: %dpx; top: %dpx; height: %dpx;"></div>`,
			endX, startY+20, endY-(startY+20)))

		// Flecha en el destino
		html.WriteString(fmt.Sprintf(`
            <div class="arrow" style="left: %dpx; top: %dpx;"></div>`,
			endX-6, endY-8))

		// Renderizar conexiones de los hijos recursivamente
		renderTreeConnections(child, html)
	}
}

// Funciones auxiliares
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// generateSuperBlockHTML genera el reporte del superbloque en HTML
func generateSuperBlockHTML(superblock structs.SuperBloque, partitionName, diskPath string) string {
	var html strings.Builder

	html.WriteString(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Reporte SUPERBLOQUE - ExtreamFS</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 800px;
            margin: 0 auto;
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            border-radius: 20px;
            padding: 30px;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
        }
        
        .header {
            text-align: center;
            margin-bottom: 40px;
            padding-bottom: 20px;
            border-bottom: 3px solid #667eea;
        }
        
        .header h1 {
            color: #2c3e50;
            font-size: 2.5rem;
            margin-bottom: 10px;
            background: linear-gradient(45deg, #667eea, #764ba2);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .header .subtitle {
            color: #7f8c8d;
            font-size: 1.1rem;
            font-weight: 300;
        }
        
        .superblock-table-container {
            background: white;
            border-radius: 15px;
            padding: 20px;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.1);
            margin-bottom: 30px;
        }
        
        .table-title {
            background: linear-gradient(135deg, #27ae60, #2ecc71);
            color: white;
            text-align: center;
            padding: 12px;
            margin: -20px -20px 20px -20px;
            border-radius: 15px 15px 0 0;
            font-weight: 700;
            font-size: 1.1rem;
            border: 3px solid #27ae60;
        }
        
        .superblock-table {
            width: 100%;
            border-collapse: collapse;
            font-family: 'Arial', sans-serif;
            font-size: 0.9rem;
        }
        
        .superblock-table td {
            padding: 8px 12px;
            border: 2px solid #27ae60;
            text-align: center;
            vertical-align: middle;
        }
        
        .superblock-table .field-name {
            background: linear-gradient(135deg, #2ecc71, #27ae60);
            color: white;
            font-weight: 600;
            width: 60%;
        }
        
        .superblock-table .field-value {
            background: #f8f9fa;
            color: #2c3e50;
            font-family: 'Courier New', monospace;
            font-weight: 500;
            width: 40%;
        }
        
        .superblock-table tr:hover .field-value {
            background: #e8f5e8;
            transition: background-color 0.3s ease;
        }
        
        .info-section {
            background: linear-gradient(135deg, #f8f9fa, #e9ecef);
            border-radius: 10px;
            padding: 20px;
            margin-bottom: 20px;
            border-left: 5px solid #667eea;
        }
        
        .info-title {
            color: #2c3e50;
            font-size: 1.2rem;
            font-weight: 600;
            margin-bottom: 15px;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        
        .info-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 10px;
        }
        
        .info-item {
            background: rgba(255, 255, 255, 0.8);
            padding: 10px;
            border-radius: 8px;
            border-left: 3px solid #3498db;
        }
        
        .info-label {
            font-size: 0.8rem;
            color: #7f8c8d;
            font-weight: 600;
            text-transform: uppercase;
        }
        
        .info-value {
            font-size: 1rem;
            color: #2c3e50;
            font-weight: 600;
            margin-top: 2px;
        }
        
        .footer {
            text-align: center;
            margin-top: 30px;
            padding-top: 20px;
            border-top: 2px solid #ecf0f1;
            color: #7f8c8d;
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 20px;
                margin: 10px;
            }
            
            .superblock-table {
                font-size: 0.8rem;
            }
            
            .superblock-table td {
                padding: 6px 8px;
            }
            
            .info-grid {
                grid-template-columns: 1fr;
            }
        }
        
        .animation-fade {
            animation: fadeInUp 0.6s ease forwards;
        }
        
        @keyframes fadeInUp {
            from {
                opacity: 0;
                transform: translateY(30px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>📋 Reporte de SUPERBLOQUE</h1>
            <p class="subtitle">Información Detallada del Superbloque - ExtreamFS </p>
        </div>`)

	// Información del disco y partición
	html.WriteString(fmt.Sprintf(`
        <div class="info-section animation-fade">
            <div class="info-title">
                <span>💾</span>
                Información de la Partición
            </div>
            <div class="info-grid">
                <div class="info-item">
                    <div class="info-label">Partición</div>
                    <div class="info-value">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Disco</div>
                    <div class="info-value">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Fecha del Reporte</div>
                    <div class="info-value">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Sistema de Archivos</div>
                    <div class="info-value">EXT2</div>
                </div>
            </div>
        </div>`, partitionName, filepath.Base(diskPath), time.Now().Format("2006-01-02 15:04:05")))

	// Tabla principal del superbloque (exactamente como en la imagen)
	html.WriteString(`
        <div class="superblock-table-container animation-fade">
            <div class="table-title">Reporte de SUPERBLOQUE</div>
            <table class="superblock-table">`)

	// Generar todas las filas de la tabla como en la imagen
	fields := []struct {
		name  string
		value string
	}{
		{"sb_nombre_hd", filepath.Base(diskPath)},
		{"sb_arbol_virtual_count", fmt.Sprintf("%d", superblock.S_inodes_count)},
		{"sb_detalle_directorio_count", fmt.Sprintf("%d", superblock.S_inodes_count)},
		{"sb_inodos_count", fmt.Sprintf("%d", superblock.S_inodes_count)},
		{"sb_bloques_count", fmt.Sprintf("%d", superblock.S_blocks_count)},
		{"sb_arbol_virtual_free", fmt.Sprintf("%d", superblock.S_free_inodes_count)},
		{"sb_detalle_directorio_free", fmt.Sprintf("%d", superblock.S_free_inodes_count)},
		{"sb_inodos_free", fmt.Sprintf("%d", superblock.S_free_inodes_count)},
		{"sb_bloques_free", fmt.Sprintf("%d", superblock.S_free_blocks_count)},
		{"sb_date_creacion", formatSuperBlockTimestamp(superblock.S_mtime)},
		{"sb_date_ultimo_montaje", formatSuperBlockTimestamp(superblock.S_umtime)},
		{"sb_montajes_count", fmt.Sprintf("%d", superblock.S_mnt_count)},
		{"sb_ap_bitmap_arbol_directorio", fmt.Sprintf("%d", superblock.S_bm_inode_start)},
		{"sb_ap_arbol_directorio", fmt.Sprintf("%d", superblock.S_inode_start)},
		{"sb_ap_bitmap_detalle_directorio", fmt.Sprintf("%d", superblock.S_bm_inode_start)},
		{"sb_ap_detalle_directorio", fmt.Sprintf("%d", superblock.S_inode_start)},
		{"sb_ap_bitmap_inodos", fmt.Sprintf("%d", superblock.S_bm_inode_start)},
		{"sb_ap_inodos", fmt.Sprintf("%d", superblock.S_inode_start)},
		{"sb_ap_bitmap_bloques", fmt.Sprintf("%d", superblock.S_bm_block_start)},
		{"sb_ap_bloques", fmt.Sprintf("%d", superblock.S_block_start)},
		{"sb_ap_log", fmt.Sprintf("%d", superblock.S_inode_start+(superblock.S_inodes_count*superblock.S_inode_s))},
		{"sb_size_struct_arbol_directorio", fmt.Sprintf("%d", superblock.S_inode_s)},
		{"sb_size_struct_detalle_directorio", fmt.Sprintf("%d", superblock.S_inode_s)},
		{"sb_size_struct_inodo", fmt.Sprintf("%d", superblock.S_inode_s)},
		{"sb_size_struct_bloque", fmt.Sprintf("%d", superblock.S_block_s)},
		{"sb_first_free_bit_arbol_directorio", fmt.Sprintf("%d", superblock.S_first_ino)},
		{"sb_first_free_bit_detalle_directorio", fmt.Sprintf("%d", superblock.S_first_ino)},
		{"sb_first_free_bit_tabla_inodos", fmt.Sprintf("%d", superblock.S_first_ino)},
		{"sb_first_free_bit_bloques", fmt.Sprintf("%d", superblock.S_first_blo)},
		{"sb_magic_num", fmt.Sprintf("%d", superblock.S_magic)},
	}

	for _, field := range fields {
		html.WriteString(fmt.Sprintf(`
                <tr>
                    <td class="field-name">%s</td>
                    <td class="field-value">%s</td>
                </tr>`, field.name, field.value))
	}

	html.WriteString(`
            </table>
        </div>`)

	// Estadísticas adicionales
	html.WriteString(fmt.Sprintf(`
        <div class="info-section animation-fade">
            <div class="info-title">
                <span>📊</span>
                Estadísticas del Sistema de Archivos
            </div>
            <div class="info-grid">
                <div class="info-item">
                    <div class="info-label">Uso de Inodos</div>
                    <div class="info-value">%.1f%% (%d/%d)</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Uso de Bloques</div>
                    <div class="info-value">%.1f%% (%d/%d)</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Tamaño Total</div>
                    <div class="info-value">%s</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Espacio Usado</div>
                    <div class="info-value">%s</div>
                </div>
            </div>
        </div>`,
		float64(superblock.S_inodes_count-superblock.S_free_inodes_count)/float64(superblock.S_inodes_count)*100,
		superblock.S_inodes_count-superblock.S_free_inodes_count, superblock.S_inodes_count,
		float64(superblock.S_blocks_count-superblock.S_free_blocks_count)/float64(superblock.S_blocks_count)*100,
		superblock.S_blocks_count-superblock.S_free_blocks_count, superblock.S_blocks_count,
		formatBytes(superblock.S_blocks_count*superblock.S_block_s),
		formatBytes((superblock.S_blocks_count-superblock.S_free_blocks_count)*superblock.S_block_s)))

	// Footer
	html.WriteString(fmt.Sprintf(`
        <div class="footer">
            <p>📋 Reporte generado por <strong>ExtreamFS </strong></p>
            <p>🕒 %s | 📦 Partición: <strong>%s</strong></p>
            <p>🔢 Magic Number: <strong>%d</strong> | 📈 Total montajes: <strong>%d</strong></p>
        </div>
    </div>
    
    <script>
        // Animaciones para las filas de la tabla
        document.addEventListener('DOMContentLoaded', function() {
            const rows = document.querySelectorAll('.superblock-table tr');
            rows.forEach((row, index) => {
                row.style.animationDelay = (index * 50) + 'ms';
                row.style.animation = 'fadeInUp 0.6s ease forwards';
            });
            
            // Efectos hover mejorados
            rows.forEach(row => {
                row.addEventListener('mouseenter', function() {
                    this.style.transform = 'scale(1.02)';
                    this.style.transition = 'transform 0.2s ease';
                });
                row.addEventListener('mouseleave', function() {
                    this.style.transform = 'scale(1)';
                });
            });
        });
    </script>
</body>
</html>`, time.Now().Format("2006-01-02 15:04:05"), partitionName, superblock.S_magic, superblock.S_mnt_count))

	return html.String()
}

// formatSuperBlockTimestamp formatea timestamps para el superbloque
func formatSuperBlockTimestamp(timestamp int64) string {
	if timestamp <= 0 {
		return "No definido"
	}

	// Validar que el timestamp esté en un rango razonable
	if timestamp < 0 || timestamp > 4102444800 {
		return fmt.Sprintf("Timestamp inválido (%d)", timestamp)
	}

	return time.Unix(timestamp, 0).Format("2006-01-02 15:04")
}

// generateFileTxt genera el contenido del reporte de archivo en formato texto
func generateFileTxt(file *os.File, superblock structs.SuperBloque, filePath, partitionName string) string {
	var content strings.Builder

	// Encabezado del reporte
	content.WriteString("==================================================\n")
	content.WriteString("              REPORTE DE ARCHIVO\n")
	content.WriteString("              ExtreamFS \n")
	content.WriteString("==================================================\n")
	content.WriteString(fmt.Sprintf("Partición: %s\n", partitionName))
	content.WriteString(fmt.Sprintf("Archivo solicitado: %s\n", filePath))
	content.WriteString(fmt.Sprintf("Fecha: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	content.WriteString("==================================================\n\n")

	// Buscar el archivo en el sistema de archivos
	fileContent, fileInfo, err := findAndReadFile(file, superblock, filePath)

	if err != nil {
		content.WriteString(fmt.Sprintf("❌ ERROR: %v\n\n", err))
		content.WriteString("==================================================\n")
		content.WriteString("           INFORMACIÓN DE DEBUG\n")
		content.WriteString("==================================================\n")
		content.WriteString("Intentando listar contenido del directorio raíz...\n\n")

		// Mostrar contenido del directorio raíz para debug
		debugContent := listRootDirectory(file, superblock)
		content.WriteString(debugContent)

		content.WriteString("\n==================================================\n")
		content.WriteString("              FIN DEL REPORTE\n")
		content.WriteString("==================================================\n")
		return content.String()
	}

	// Mostrar información del archivo encontrado
	content.WriteString("📄 ARCHIVO ENCONTRADO\n")
	content.WriteString("==================================================\n")
	content.WriteString(fmt.Sprintf("Nombre: %s\n", fileInfo.Name))
	content.WriteString(fmt.Sprintf("Tamaño: %d bytes\n", fileInfo.Size))
	content.WriteString(fmt.Sprintf("Tipo: %s\n", fileInfo.Type))
	content.WriteString(fmt.Sprintf("Inodo: %d\n", fileInfo.InodeIndex))
	content.WriteString(fmt.Sprintf("UID: %d\n", fileInfo.UID))
	content.WriteString(fmt.Sprintf("Permisos: %s\n", fileInfo.Permissions))
	content.WriteString(fmt.Sprintf("Fecha acceso: %s\n", fileInfo.AccessTime))
	content.WriteString(fmt.Sprintf("Fecha creación: %s\n", fileInfo.CreationTime))
	content.WriteString(fmt.Sprintf("Fecha modificación: %s\n", fileInfo.ModificationTime))
	content.WriteString("==================================================\n\n")

	// Mostrar contenido del archivo
	content.WriteString("📝 CONTENIDO DEL ARCHIVO\n")
	content.WriteString("==================================================\n")

	if len(fileContent) == 0 {
		content.WriteString("(El archivo está vacío)\n")
	} else {
		// Verificar si el contenido es texto o binario
		if isTextContent(fileContent) {
			content.WriteString(fileContent)
		} else {
			content.WriteString("⚠️  El archivo contiene datos binarios.\n")
			content.WriteString("Mostrando representación hexadecimal:\n\n")

			// Mostrar contenido en formato hexadecimal
			hexContent := formatAsHex(fileContent)
			content.WriteString(hexContent)
		}
	}

	content.WriteString("\n\n==================================================\n")
	content.WriteString("              FIN DEL ARCHIVO\n")
	content.WriteString("==================================================\n")
	content.WriteString(fmt.Sprintf("Total de bytes leídos: %d\n", len(fileContent)))
	content.WriteString(fmt.Sprintf("Archivo: %s\n", filePath))
	content.WriteString("==================================================\n")

	return content.String()
}

// Estructura para información del archivo
type FileInfo struct {
	Name             string
	Size             int64
	Type             string
	InodeIndex       int
	UID              int64
	Permissions      string
	AccessTime       string
	CreationTime     string
	ModificationTime string
}

// findAndReadFile busca y lee un archivo específico en el sistema de archivos
func findAndReadFile(file *os.File, superblock structs.SuperBloque, targetPath string) (string, *FileInfo, error) {
	// Normalizar la ruta (remover / inicial si existe)
	targetPath = strings.TrimPrefix(targetPath, "/")

	// Si la ruta está vacía, es un error
	if targetPath == "" {
		return "", nil, fmt.Errorf("ruta de archivo vacía")
	}

	// Dividir la ruta en componentes
	pathComponents := strings.Split(targetPath, "/")

	// Buscar desde el directorio raíz
	currentInodeIndex := int64(0) // Comenzar desde el inodo raíz

	// Leer todos los inodos
	inodes := readInodesFromPartition(file, superblock)
	if len(inodes) == 0 {
		return "", nil, fmt.Errorf("no se encontraron inodos en la partición")
	}

	// Navegar por cada componente de la ruta
	for i, component := range pathComponents {
		if component == "" {
			continue // Saltar componentes vacíos
		}

		// Verificar que el inodo actual sea válido
		if currentInodeIndex >= int64(len(inodes)) {
			return "", nil, fmt.Errorf("inodo índice %d fuera de rango", currentInodeIndex)
		}

		currentInode := inodes[currentInodeIndex]

		// Verificar que sea un directorio (excepto para el último componente)
		isLastComponent := (i == len(pathComponents)-1)
		if !isLastComponent {
			var realType int64
			if currentInode.I_type >= 48 && currentInode.I_type <= 57 {
				realType = int64(currentInode.I_type - 48)
			} else {
				realType = int64(currentInode.I_type)
			}

			if realType != 0 && currentInode.I_s != 96 {
				return "", nil, fmt.Errorf("componente '%s' no es un directorio", component)
			}
		}

		// Usar la función de file_operations.go - CAMBIO AQUÍ
		foundInodeIndex, err := findInodeInDirectory(file, &superblock, currentInodeIndex, component)
		if err != nil {
			return "", nil, fmt.Errorf("error buscando '%s': %v", component, err)
		}

		if foundInodeIndex == -1 {
			return "", nil, fmt.Errorf("archivo/directorio '%s' no encontrado en la ruta '%s'", component, targetPath)
		}

		// Para el último componente, verificar que sea un archivo
		if isLastComponent {
			// Verificar si es directorio
			foundInode := inodes[foundInodeIndex]
			var realType int64
			if foundInode.I_type >= 48 && foundInode.I_type <= 57 {
				realType = int64(foundInode.I_type - 48)
			} else {
				realType = int64(foundInode.I_type)
			}

			isDirectory := (realType == 0 || foundInode.I_s == 96)
			if isDirectory {
				return "", nil, fmt.Errorf("'%s' es un directorio, no un archivo", targetPath)
			}

			// Leer el contenido del archivo usando la función de file_operations.go
			fileContent, err := readFileContentMultiBlock(file, &superblock, &foundInode)
			if err != nil {
				return "", nil, fmt.Errorf("error leyendo contenido del archivo: %v", err)
			}

			// Crear información del archivo
			fileInfo := &FileInfo{
				Name:             component,
				Size:             foundInode.I_s,
				Type:             "Archivo regular",
				InodeIndex:       int(foundInodeIndex),
				UID:              foundInode.I_uid,
				Permissions:      formatPermissions(foundInode.I_perm),
				AccessTime:       formatTimestamp(foundInode.I_atime),
				CreationTime:     formatTimestamp(foundInode.I_ctime),
				ModificationTime: formatTimestamp(foundInode.I_mtime),
			}

			return fileContent, fileInfo, nil
		}

		// Continuar con el siguiente nivel
		currentInodeIndex = foundInodeIndex
	}

	return "", nil, fmt.Errorf("ruta de archivo inválida")
}

// isTextContent verifica si el contenido es texto legible
func isTextContent(content string) bool {
	if len(content) == 0 {
		return true
	}

	// Contar caracteres no imprimibles
	nonPrintable := 0
	for _, char := range content {
		if char < 32 && char != 9 && char != 10 && char != 13 { // Excluir tab, LF, CR
			nonPrintable++
		}
	}

	// Si más del 20% son caracteres no imprimibles, considerarlo binario
	threshold := len(content) / 5
	return nonPrintable <= threshold
}

// formatAsHex formatea contenido binario como hexadecimal
func formatAsHex(content string) string {
	var hex strings.Builder

	bytes := []byte(content)
	for i := 0; i < len(bytes); i += 16 {
		// Dirección
		hex.WriteString(fmt.Sprintf("%08X: ", i))

		// Bytes en hexadecimal
		for j := 0; j < 16; j++ {
			if i+j < len(bytes) {
				hex.WriteString(fmt.Sprintf("%02X ", bytes[i+j]))
			} else {
				hex.WriteString("   ")
			}
		}

		// Separador
		hex.WriteString(" | ")

		// Representación ASCII
		for j := 0; j < 16 && i+j < len(bytes); j++ {
			b := bytes[i+j]
			if b >= 32 && b <= 126 {
				hex.WriteByte(b)
			} else {
				hex.WriteByte('.')
			}
		}

		hex.WriteString("\n")

		// Limitar salida para archivos muy grandes
		if i > 1024 {
			hex.WriteString("... (contenido truncado, archivo muy grande)\n")
			break
		}
	}

	return hex.String()
}

// listRootDirectory lista el contenido del directorio raíz para debug
func listRootDirectory(file *os.File, superblock structs.SuperBloque) string {
	var content strings.Builder

	inodes := readInodesFromPartition(file, superblock)
	if len(inodes) == 0 {
		return "No se pudieron leer los inodos.\n"
	}

	// Buscar directorios válidos
	content.WriteString("Directorios encontrados:\n")

	for i, inode := range inodes {
		if !isInodeValid(inode) {
			continue
		}

		var realType int64
		if inode.I_type >= 48 && inode.I_type <= 57 {
			realType = int64(inode.I_type - 48)
		} else {
			realType = int64(inode.I_type)
		}

		if realType == 0 || inode.I_s == 96 {
			content.WriteString(fmt.Sprintf("\nDirectorio en inodo %d:\n", i))

			// Listar contenido del directorio
			for _, blockIndex := range inode.I_block {
				if blockIndex == -1 || blockIndex < 0 {
					continue
				}

				blockPosition := superblock.S_block_start + (blockIndex * superblock.S_block_s)
				file.Seek(blockPosition, 0)

				var folderBlock structs.BloqueCarpeta
				if err := binary.Read(file, binary.LittleEndian, &folderBlock); err != nil {
					continue
				}

				for _, entry := range folderBlock.BContent {
					entryName := strings.TrimRight(string(entry.BName[:]), "\x00")
					if entryName != "" {
						content.WriteString(fmt.Sprintf("  - %s (inodo %d)\n", entryName, entry.BInodo))
					}
				}
			}
		}
	}

	return content.String()
}

// Estructura para almacenar información de archivos/directorios
type LsEntry struct {
	Permissions      string
	Owner            string
	Group            string
	Size             int64
	CreationDate     string
	CreationTime     string
	ModificationDate string
	ModificationTime string
	Type             string
	Name             string
	InodeIndex       int64
}

// generateLsHTML genera el reporte de listado en HTML
func generateLsHTML(file *os.File, superblock structs.SuperBloque, dirPath, partitionName string) string {
	var html strings.Builder

	html.WriteString(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Reporte LS - ExtreamFS</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 1400px;
            margin: 0 auto;
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            border-radius: 20px;
            padding: 30px;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
        }
        
        .header {
            text-align: center;
            margin-bottom: 40px;
            padding-bottom: 20px;
            border-bottom: 3px solid #667eea;
        }
        
        .header h1 {
            color: #2c3e50;
            font-size: 2.5rem;
            margin-bottom: 10px;
            background: linear-gradient(45deg, #667eea, #764ba2);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .header .subtitle {
            color: #7f8c8d;
            font-size: 1.1rem;
            font-weight: 300;
        }
        
        .path-info {
            background: linear-gradient(135deg, #f8f9fa, #e9ecef);
            border-radius: 10px;
            padding: 15px 20px;
            margin-bottom: 25px;
            border-left: 5px solid #3498db;
        }
        
        .path-info h3 {
            color: #2c3e50;
            margin-bottom: 5px;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        
        .path-value {
            color: #7f8c8d;
            font-family: 'Courier New', monospace;
            font-size: 1.1rem;
        }
        
        .ls-table-container {
            background: white;
            border-radius: 15px;
            padding: 0;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.1);
            overflow: hidden;
            margin-bottom: 30px;
        }
        
        .ls-table {
            width: 100%;
            border-collapse: collapse;
            font-family: 'Arial', sans-serif;
            font-size: 0.9rem;
        }
        
        .ls-table th {
            background: linear-gradient(135deg, #34495e, #2c3e50);
            color: white;
            padding: 15px 12px;
            text-align: center;
            font-weight: 600;
            font-size: 0.9rem;
            border-right: 1px solid rgba(255, 255, 255, 0.1);
        }
        
        .ls-table th:last-child {
            border-right: none;
        }
        
        .ls-table td {
            padding: 12px;
            text-align: center;
            border-bottom: 1px solid #ecf0f1;
            vertical-align: middle;
            transition: background-color 0.3s ease;
        }
        
        .ls-table tr:hover td {
            background: rgba(52, 152, 219, 0.05);
        }
        
        .ls-table tr:nth-child(even) td {
            background: rgba(248, 249, 250, 0.8);
        }
        
        .ls-table tr:nth-child(even):hover td {
            background: rgba(52, 152, 219, 0.1);
        }
        
        .permissions-cell {
            font-family: 'Courier New', monospace;
            background: rgba(52, 152, 219, 0.1);
            border-radius: 5px;
            padding: 4px 8px;
            font-weight: 600;
            color: #2c3e50;
        }
        
        .owner-cell {
            font-weight: 600;
            color: #27ae60;
        }
        
        .group-cell {
            font-weight: 600;
            color: #f39c12;
        }
        
        .size-cell {
            font-family: 'Courier New', monospace;
            font-weight: 600;
            color: #8e44ad;
        }
        
        .date-cell {
            font-family: 'Courier New', monospace;
            color: #7f8c8d;
            font-size: 0.85rem;
        }
        
        .time-cell {
            font-family: 'Courier New', monospace;
            color: #7f8c8d;
            font-size: 0.85rem;
        }
        
        .type-cell {
            font-weight: 600;
            padding: 4px 12px;
            border-radius: 15px;
            color: white;
            font-size: 0.85rem;
        }
        
        .type-archivo {
            background: linear-gradient(135deg, #e74c3c, #c0392b);
        }
        
        .type-carpeta {
            background: linear-gradient(135deg, #3498db, #2980b9);
        }
        
        .name-cell {
            font-weight: 600;
            color: #2c3e50;
            text-align: left;
            padding-left: 20px;
        }
        
        .name-file {
            color: #e74c3c;
        }
        
        .name-folder {
            color: #3498db;
        }
        
        .empty-state {
            text-align: center;
            padding: 60px 20px;
            color: #7f8c8d;
        }
        
        .empty-icon {
            font-size: 4rem;
            margin-bottom: 20px;
            opacity: 0.7;
        }
        
        .stats-section {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        
        .stat-item {
            background: linear-gradient(135deg, #f8f9fa, #e9ecef);
            padding: 20px;
            border-radius: 10px;
            text-align: center;
            border-left: 5px solid #3498db;
        }
        
        .stat-number {
            font-size: 2rem;
            font-weight: 700;
            color: #2c3e50;
            margin-bottom: 5px;
        }
        
        .stat-label {
            color: #7f8c8d;
            font-weight: 600;
            text-transform: uppercase;
            font-size: 0.9rem;
        }
        
        .footer {
            text-align: center;
            margin-top: 30px;
            padding-top: 20px;
            border-top: 2px solid #ecf0f1;
            color: #7f8c8d;
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 20px;
                margin: 10px;
            }
            
            .ls-table {
                font-size: 0.8rem;
            }
            
            .ls-table th, .ls-table td {
                padding: 8px 6px;
            }
            
            .stats-section {
                grid-template-columns: 1fr;
            }
        }
        
        .animation-fade {
            animation: fadeInUp 0.6s ease forwards;
        }
        
        @keyframes fadeInUp {
            from {
                opacity: 0;
                transform: translateY(30px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>📋 Reporte LS</h1>
            <p class="subtitle">Listado de Archivos y Directorios - ExtreamFS </p>
        </div>`)

	// Información de la ruta
	html.WriteString(fmt.Sprintf(`
        <div class="path-info animation-fade">
            <h3>
                <span>📁</span>
                Directorio listado:
            </h3>
            <div class="path-value">%s</div>
        </div>`, dirPath))

	// Obtener entradas del directorio
	entries, err := listDirectoryContents(file, superblock, dirPath)

	if err != nil {
		html.WriteString(fmt.Sprintf(`
        <div class="empty-state animation-fade">
            <div class="empty-icon">❌</div>
            <h3>Error al listar directorio</h3>
            <p>%v</p>
        </div>`, err))
	} else if len(entries) == 0 {
		html.WriteString(`
        <div class="empty-state animation-fade">
            <div class="empty-icon">📂</div>
            <h3>Directorio vacío</h3>
            <p>No se encontraron archivos o directorios en esta ubicación.</p>
        </div>`)
	} else {
		// Estadísticas
		fileCount := 0
		dirCount := 0
		totalSize := int64(0)

		for _, entry := range entries {
			if entry.Type == "Archivo" {
				fileCount++
				totalSize += entry.Size
			} else {
				dirCount++
			}
		}

		html.WriteString(fmt.Sprintf(`
        <div class="stats-section animation-fade">
            <div class="stat-item">
                <div class="stat-number">%d</div>
                <div class="stat-label">Total Items</div>
            </div>
            <div class="stat-item">
                <div class="stat-number">%d</div>
                <div class="stat-label">Archivos</div>
            </div>
            <div class="stat-item">
                <div class="stat-number">%d</div>
                <div class="stat-label">Directorios</div>
            </div>
            <div class="stat-item">
                <div class="stat-number">%s</div>
                <div class="stat-label">Tamaño Total</div>
            </div>
        </div>`, len(entries), fileCount, dirCount, formatBytes(totalSize)))

		// Tabla de archivos y directorios (exactamente como en la imagen)
		html.WriteString(`
        <div class="ls-table-container animation-fade">
            <table class="ls-table">
                <thead>
                    <tr>
                        <th>Permisos</th>
                        <th>Owner</th>
                        <th>Grupo</th>
                        <th>Size (en bytes)</th>
                        <th>Fecha</th>
                        <th>Hora</th>
                        <th>Tipo</th>
                        <th>Name</th>
                    </tr>
                </thead>
                <tbody>`)

		// Generar filas de la tabla
		for _, entry := range entries {
			typeClass := "type-archivo"
			nameClass := "name-file"
			if entry.Type == "Carpeta" {
				typeClass = "type-carpeta"
				nameClass = "name-folder"
			}

			html.WriteString(fmt.Sprintf(`
                    <tr>
                        <td><span class="permissions-cell">%s</span></td>
                        <td class="owner-cell">%s</td>
                        <td class="group-cell">%s</td>
                        <td class="size-cell">%d</td>
                        <td class="date-cell">%s</td>
                        <td class="time-cell">%s</td>
                        <td><span class="type-cell %s">%s</span></td>
                        <td class="name-cell %s">%s</td>
                    </tr>`,
				entry.Permissions, entry.Owner, entry.Group, entry.Size,
				entry.ModificationDate, entry.ModificationTime,
				typeClass, entry.Type, nameClass, entry.Name))
		}

		html.WriteString(`
                </tbody>
            </table>
        </div>`)
	}

	// Footer
	html.WriteString(fmt.Sprintf(`
        <div class="footer">
            <p>📋 Reporte generado por <strong>ExtreamFS </strong></p>
            <p>🕒 %s | 📦 Partición: <strong>%s</strong></p>
            <p>📁 Directorio: <strong>%s</strong> | 📊 Total de elementos: <strong>%d</strong></p>
        </div>
    </div>
    
    <script>
        // Animaciones para las filas de la tabla
        document.addEventListener('DOMContentLoaded', function() {
            const rows = document.querySelectorAll('.ls-table tbody tr');
            rows.forEach((row, index) => {
                row.style.animationDelay = (index * 50) + 'ms';
                row.style.animation = 'fadeInUp 0.6s ease forwards';
            });
            
            // Efectos hover mejorados
            rows.forEach(row => {
                row.addEventListener('mouseenter', function() {
                    this.style.transform = 'scale(1.01)';
                    this.style.transition = 'transform 0.2s ease';
                });
                row.addEventListener('mouseleave', function() {
                    this.style.transform = 'scale(1)';
                });
            });
        });
    </script>
</body>
</html>`, time.Now().Format("2006-01-02 15:04:05"), partitionName, dirPath, len(entries)))

	return html.String()
}

// listDirectoryContents lista el contenido de un directorio específico
func listDirectoryContents(file *os.File, superblock structs.SuperBloque, dirPath string) ([]LsEntry, error) {
	var entries []LsEntry

	// Normalizar la ruta
	if dirPath == "" || dirPath == "/" {
		dirPath = "/"
	} else {
		dirPath = strings.TrimPrefix(dirPath, "/")
	}

	// Buscar el directorio objetivo
	targetInodeIndex, err := findDirectoryInode(file, superblock, dirPath)
	if err != nil {
		return entries, fmt.Errorf("directorio no encontrado: %v", err)
	}

	// Leer todos los inodos
	inodes := readInodesFromPartition(file, superblock)
	if targetInodeIndex >= int64(len(inodes)) {
		return entries, fmt.Errorf("índice de inodo fuera de rango")
	}

	dirInode := inodes[targetInodeIndex]

	// Verificar que sea un directorio
	var realType int64
	if dirInode.I_type >= 48 && dirInode.I_type <= 57 {
		realType = int64(dirInode.I_type - 48)
	} else {
		realType = int64(dirInode.I_type)
	}

	if realType != 0 && dirInode.I_s != 96 {
		return entries, fmt.Errorf("la ruta especificada no es un directorio")
	}

	// Leer el contenido del directorio
	for _, blockIndex := range dirInode.I_block {
		if blockIndex == -1 || blockIndex < 0 {
			continue
		}

		// Leer bloque de directorio
		blockPosition := superblock.S_block_start + (blockIndex * superblock.S_block_s)
		file.Seek(blockPosition, 0)

		var folderBlock structs.BloqueCarpeta
		if err := binary.Read(file, binary.LittleEndian, &folderBlock); err != nil {
			continue
		}

		// Procesar cada entrada del directorio
		for _, entry := range folderBlock.BContent {
			entryName := strings.TrimRight(string(entry.BName[:]), "\x00")

			// Saltar entradas vacías y referencias a directorio actual/padre
			if entryName == "" || entryName == "." || entryName == ".." {
				continue
			}

			if entry.BInodo >= 0 && int(entry.BInodo) < len(inodes) {
				entryInode := inodes[entry.BInodo]

				// Crear entrada LS
				lsEntry := createLsEntry(entryInode, entryName, int64(entry.BInodo))
				entries = append(entries, lsEntry)
			}
		}
	}

	return entries, nil
}

// findDirectoryInode encuentra el inodo de un directorio específico
func findDirectoryInode(file *os.File, superblock structs.SuperBloque, dirPath string) (int64, error) {
	if dirPath == "/" {
		// Para el directorio raíz, buscar el primer directorio válido
		inodes := readInodesFromPartition(file, superblock)
		for i, inode := range inodes {
			if !isInodeValid(inode) {
				continue
			}

			var realType int64
			if inode.I_type >= 48 && inode.I_type <= 57 {
				realType = int64(inode.I_type - 48)
			} else {
				realType = int64(inode.I_type)
			}

			if realType == 0 || inode.I_s == 96 {
				return int64(i), nil
			}
		}
		return -1, fmt.Errorf("no se encontró directorio raíz")
	}

	// Para otros directorios, navegar desde la raíz
	pathComponents := strings.Split(dirPath, "/")
	currentInodeIndex := int64(0)

	// Buscar directorio raíz primero
	rootIndex, err := findDirectoryInode(file, superblock, "/")
	if err != nil {
		return -1, err
	}
	currentInodeIndex = rootIndex

	// Navegar por cada componente
	for _, component := range pathComponents {
		if component == "" {
			continue
		}

		foundIndex, err := findInodeInDirectory(file, &superblock, currentInodeIndex, component)
		if err != nil {
			return -1, fmt.Errorf("no se encontró directorio '%s': %v", component, err)
		}

		if foundIndex == -1 {
			return -1, fmt.Errorf("directorio '%s' no encontrado", component)
		}

		currentInodeIndex = foundIndex
	}

	return currentInodeIndex, nil
}

// createLsEntry crea una entrada LS a partir de un inodo
func createLsEntry(inode structs.Inodos, name string, inodeIndex int64) LsEntry {
	// Determinar el tipo
	var realType int64
	if inode.I_type >= 48 && inode.I_type <= 57 {
		realType = int64(inode.I_type - 48)
	} else {
		realType = int64(inode.I_type)
	}

	entryType := "Archivo"
	if realType == 0 || inode.I_s == 96 {
		entryType = "Carpeta"
	}

	// Formatear fechas
	modTime := time.Unix(inode.I_mtime, 0)
	creTime := time.Unix(inode.I_ctime, 0)

	// Generar nombres de usuario y grupo (simulados)
	ownerName := fmt.Sprintf("User%d", inode.I_uid)
	groupName := "Mi grupo"
	if inode.I_uid != 1 {
		groupName = "Otro grupo"
	}

	return LsEntry{
		Permissions:      formatLsPermissions(inode.I_perm),
		Owner:            ownerName,
		Group:            groupName,
		Size:             inode.I_s,
		CreationDate:     creTime.Format("02/01/2006"),
		CreationTime:     creTime.Format("15:04"),
		ModificationDate: modTime.Format("02/01/2006"),
		ModificationTime: modTime.Format("15:04"),
		Type:             entryType,
		Name:             name,
		InodeIndex:       inodeIndex,
	}
}

// formatLsPermissions formatea permisos en formato ls (-rwxrwxrwx)
func formatLsPermissions(perm [3]byte) string {
	// Convertir permisos a formato legible
	permissions := "-"

	// Determinar el tipo (siempre archivo o directorio aquí)
	// El primer carácter se determinará por el contexto

	// Permisos del propietario (usuario)
	ownerPerm := int(perm[0])
	if ownerPerm >= 48 && ownerPerm <= 55 { // ASCII '0'-'7'
		ownerPerm = ownerPerm - 48
	}

	if ownerPerm&4 != 0 {
		permissions += "r"
	} else {
		permissions += "-"
	}
	if ownerPerm&2 != 0 {
		permissions += "w"
	} else {
		permissions += "-"
	}
	if ownerPerm&1 != 0 {
		permissions += "x"
	} else {
		permissions += "-"
	}

	// Permisos del grupo
	groupPerm := int(perm[1])
	if groupPerm >= 48 && groupPerm <= 55 {
		groupPerm = groupPerm - 48
	}

	if groupPerm&4 != 0 {
		permissions += "r"
	} else {
		permissions += "-"
	}
	if groupPerm&2 != 0 {
		permissions += "w"
	} else {
		permissions += "-"
	}
	if groupPerm&1 != 0 {
		permissions += "x"
	} else {
		permissions += "-"
	}

	// Permisos de otros
	otherPerm := int(perm[2])
	if otherPerm >= 48 && otherPerm <= 55 {
		otherPerm = otherPerm - 48
	}

	if otherPerm&4 != 0 {
		permissions += "r"
	} else {
		permissions += "-"
	}
	if otherPerm&2 != 0 {
		permissions += "w"
	} else {
		permissions += "-"
	}
	if otherPerm&1 != 0 {
		permissions += "x"
	} else {
		permissions += "-"
	}

	return permissions
}
