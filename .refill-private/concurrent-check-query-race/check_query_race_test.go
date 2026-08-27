package concurrentcheckqueryrace

import (
	"cave-archive/internal/application"
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"sync"
	"testing"
)

func TestConcurrentCheckRunQueriesAreRaceFree(t *testing.T) {
	st, err := store.New("")
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	archive, err := service.Create("RACE-QUERY-1", "并发洞", "2026-01-01", "CGCS2000", "create-race-query")
	if err != nil {
		t.Fatal(err)
	}
	revision := &domain.Revision{
		Stations:    []domain.Station{{ID: "s1", Name: "入口"}},
		SubmittedBy: "测绘员",
	}
	archive, err = service.AddRevision(archive.ID, revision, archive.Version)
	if err != nil {
		t.Fatal(err)
	}
	archive, err = service.Submit(archive.ID, archive.Version)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Check(archive.ID, archive.Version)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 32
	const rounds = 40
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for round := 0; round < rounds; round++ {
				if worker%2 == 0 {
					if _, queryErr := service.CheckRun(archive.ID, result.Run.ID); queryErr != nil {
						errs <- queryErr
					}
				} else if _, queryErr := service.Detail(archive.ID); queryErr != nil {
					errs <- queryErr
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("并发质检查询返回错误: %v", err)
	}
}
