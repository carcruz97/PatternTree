# PatternTree (MVP)

Herramienta de línea de comandos en Go que recorre una carpeta y genera un informe en texto con:

- tipo de cada archivo y resumen de los que no se muestran;
- estructura de los JSON (claves y tipos);
- imports usados, sin usar o no verificables;
- funciones, clases y métodos, con una estimación de complejidad;
- líneas de código separadas en SDK/API, lógica y logs;
- árbol de carpetas y entornos de desarrollo ignorados.

Solo usa la librería estándar de Go.

## Requisitos

- Go 1.22 o superior (usa `range` sobre enteros y la función `max` integrada).

## Uso

```bash
go build -o patterntree .
./patterntree [carpeta]
```

Sin argumento analiza el directorio actual. Si la ruta no existe, termina con código 1.

## Qué analiza

| Tipo | Extensiones | Qué informa |
|---|---|---|
| Datos | `.csv`, `.pkl`, `.pickle`, `.joblib` | tipo |
| JSON | `.json` | claves y tipos; marca los JSON inválidos |
| Python | `.py` | imports, símbolos, componentes |
| JavaScript / TypeScript | `.js`, `.mjs`, `.cjs`, `.jsx`, `.ts`, `.tsx` | imports, símbolos, componentes |
| Documentación | `.md` | tipo |

Otros archivos se agrupan por extensión en "Otros archivos (no mostrados)". Las carpetas de entorno (`venv`, `.venv`, `node_modules`, `__pypackages__`, entornos de conda, etc.) no se recorren: se cuentan aparte.

### Complejidad

Se estima con la profundidad de bucles anidados (`for`, `while`, `forEach`, `map`, `filter`, `reduce`):

| Bucles anidados | Lectura |
|---|---|
| 0 | O(1) |
| 1 | O(n) |
| 2 | O(n²), con alerta |
| 3 o más | O(n^k), con alerta fuerte |

### Componentes

Cada línea de código se clasifica así, en este orden de prioridad:

1. **Logs:** `print`, `logging`/`logger`, `console.*`.
2. **SDK + API:** paquetes de Apify (`apify`, `apify_client`, `apify-client`), librerías HTTP (`httpx`, `requests`, `aiohttp`, `fastapi`, `axios`, `got`, `node-fetch`), rutas y llamadas HTTP con URL literal, y `fetch`.
3. **Lógica:** el resto.

## Estructura del código

| Archivo | Responsabilidad |
|---|---|
| `main.go` | Punto de entrada: lee la ruta, lanza el escaneo e imprime el informe. |
| `scanner.go` | Recorre el árbol en paralelo, arma el árbol visual y cuenta los entornos. |
| `detector.go` | Inspecciona cada archivo: tipo, imports y componentes. |
| `symbols.go` | Detecta funciones, clases y métodos de Python y JS con expresiones regulares. |
| `big-o.go` | Estima la profundidad de bucles anidados. |
| `components.go` | Separa las líneas en SDK/API, lógica y logs. |
| `jsonschema.go` | Extrae las claves y los tipos de un JSON. |

## Limitaciones del MVP

- El análisis usa expresiones regulares, no un AST: puede fallar con código poco común o mal formateado.
- La complejidad es una heurística basada en bucles anidados. No considera recursión ni llamadas a otras funciones.
- El tipo de archivo se decide solo por la extensión.
- Los archivos se procesan en paralelo, así que el orden del informe no es fijo.
- `reportsJSON` y `symbolsJSON` están implementados, pero `main.go` no los usa todavía.
- No hay `go.mod` ni tests en esta versión. Para compilar, ejecutar `go mod init patterntree` en la carpeta antes de `go build`.
