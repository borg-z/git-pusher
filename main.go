package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-git/go-git/v5" // with go modules disabled
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/xanzy/go-gitlab"
)

func clone(name string, uri string) {
	log.Println("Clone: ", name)
	git.PlainClone("repos/"+name, false, &git.CloneOptions{
		URL: uri,
	})
	log.Println("Copy from template: ", name)
	CopyDirectory("template/", "repos/"+name)
	CommitPush("repos/" + name)

}

func CommitPush(path string) {
	r, _ := git.PlainOpen(path)
	w, _ := r.Worktree()
	_, err := w.Add(".")
	if err != nil {
		log.Println("Git add error: ", err, path)
	}
	_, err = w.Commit("Go push faster", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Git the Pusher",
			Email: "git@push.org",
			When:  time.Now(),
		},
	})
	if err != nil {
		log.Println("Git commit error: ", err, path)
	}
	r.Push(&git.PushOptions{})

}

func CopyDirectory(scrDir, dest string) error {
	entries, err := ioutil.ReadDir(scrDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(scrDir, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		fileInfo, err := os.Stat(sourcePath)
		if err != nil {
			return err
		}

		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("failed to get raw syscall.Stat_t data for '%s'", sourcePath)
		}

		switch fileInfo.Mode() & os.ModeType {
		case os.ModeDir:
			if err := CreateIfNotExists(destPath, 0755); err != nil {
				return err
			}
			if err := CopyDirectory(sourcePath, destPath); err != nil {
				return err
			}
		case os.ModeSymlink:
			if err := CopySymLink(sourcePath, destPath); err != nil {
				return err
			}
		default:
			if err := Copy(sourcePath, destPath); err != nil {
				return err
			}
		}

		if err := os.Lchown(destPath, int(stat.Uid), int(stat.Gid)); err != nil {
			return err
		}

		isSymlink := entry.Mode()&os.ModeSymlink != 0
		if !isSymlink {
			if err := os.Chmod(destPath, entry.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func Copy(srcFile, dstFile string) error {
	out, err := os.Create(dstFile)
	if err != nil {
		return err
	}

	defer out.Close()

	in, err := os.Open(srcFile)
	defer in.Close()
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return nil
}

func Exists(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}

	return true
}

func CreateIfNotExists(dir string, perm os.FileMode) error {
	if Exists(dir) {
		return nil
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return fmt.Errorf("failed to create directory: '%s', error: '%s'", dir, err.Error())
	}

	return nil
}

func CopySymLink(source, dest string) error {
	link, err := os.Readlink(source)
	if err != nil {
		return err
	}
	return os.Symlink(link, dest)
}

func main() {

	groupFlag := flag.String("groupid", "238", "id of the group of repositories")
	tokenFlag := flag.String("token", "", "Private token")
	urlFlag := flag.String("base_url", "", "https://gitlab-ci.local/api/v4")

	flag.Parse()
	groupid := *groupFlag
	baseUrl := *urlFlag
	token := *tokenFlag

	gl, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseUrl))
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	opt := &gitlab.ListGroupProjectsOptions{}

	maxGoroutines := 2
	guard := make(chan struct{}, maxGoroutines)

	projects, _, err := gl.Groups.ListGroupProjects(groupid, opt)
	for _, project := range projects {
		r := rand.Intn(500)
		time.Sleep(time.Duration(r) * time.Millisecond)
		guard <- struct{}{}
		go func(n string) {
			clone(project.Name, project.SSHURLToRepo)
			<-guard
		}(project.Name)

	}

}
