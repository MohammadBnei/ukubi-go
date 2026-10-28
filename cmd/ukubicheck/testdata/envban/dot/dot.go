// Package dot dot-imports os and uses only Args: the import alone is refused.
package dot

import . "os" // want `^import \. "os":`

var _ = Args
