package structs

// BloqueCarpeta representa un bloque que contiene información de directorios
type BloqueCarpeta struct {
    BContent [4]BContent // 4 entradas por bloque de carpeta
}

// BloqueArchivo representa un bloque que contiene datos de archivos  
type BloqueArchivo struct {
    BContent [64]byte // 64 bytes de datos por bloque
}

// BloqueApuntador representa un bloque que contiene punteros a otros bloques
type BloqueApuntador struct {
    BPointers [8]int64 // 8 punteros × 8 bytes = 64 bytes
}

// BContent representa una entrada en un directorio
type BContent struct {
    BName  [12]byte // Nombre del archivo/directorio
    _      [4]byte  // Padding para alineación
    BInodo int64    // Número de inodo (mantener int64 para compatibilidad)
}