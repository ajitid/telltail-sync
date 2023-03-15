// not using these, but keeping them for posterity sake
import "os/exec"

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
