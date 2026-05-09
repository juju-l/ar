package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
)

func main() {
	
	f, err := os.Create("qwe_workspace.tar.gz")
	if err != nil {
		panic(err)
	}
	defer f.Close()


		outFile, err := os.Create("qwe_workspace.tar.gz")
	if err != nil {
		fmt.Println(err)
	}
	defer outFile.Close()

	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()


	err = path2TarGzWalk("qwe", "workspace", tarWriter, []string{},  []string{".git/**"})
	if err != nil {
		fmt.Println(err)
	}
}
