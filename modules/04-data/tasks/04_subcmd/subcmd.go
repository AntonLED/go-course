//go:build !solution

package subcmd

import "io"

// Run — точка входа, main сводится к os.Exit(Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)).
//
// Глобальные флаги (до команды): -json. Команды — см. Usage и README.
// Результаты — только в stdout, ошибки и справка — только в stderr.
// Возвращает ExitOK, ExitFailure или ExitUsage.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// TODO: глобальный FlagSet, затем switch по имени команды,
	// у каждой команды — свой FlagSet.
	return -1
}
