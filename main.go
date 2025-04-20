package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Movie represents a movie with its URL and rating
type Movie struct {
	URL    string
	Rating int
}

// MovieWithRatings represents a movie with multiple ratings
type MovieWithRatings struct {
	URL     string
	Ratings []int
}

// Result represents the processed movie data for display
type Result struct {
	AvgRating float64
	VoteCount int
	URL       string
	Ratings   []int
}

// Helper functions for calculations
func avg(list []int) float64 {
	if len(list) == 0 {
		return 0
	}
	sum := 0
	for _, v := range list {
		sum += v
	}
	return float64(sum) / float64(len(list))
}

func leastSquare(list []int) float64 {
	if len(list) == 0 {
		return 0
	}
	sum := 0
	for _, v := range list {
		sum += v * v
	}
	return math.Sqrt(float64(sum) / float64(len(list)))
}

func weighted(list []int) float64 {
	if len(list) == 0 {
		return 0
	}
	weights := []int{100, 95, 80, 65, 40, 20, 5, 0, 0, 0}
	wList := make([]int, 0, len(list))
	for _, i := range list {
		if i >= 0 && i < len(weights) {
			wList = append(wList, weights[i])
		}
	}
	return avg(wList)
}

// Letterboxd represents the main application
type Letterboxd struct {
	User     string
	Friends  []string
	MyMovies []string
	Movies   []Movie
}

// getPage fetches and parses a web page
func getPage(url string) (*goquery.Document, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for retry := 0; retry < 10; retry++ {
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				return goquery.NewDocumentFromReader(resp.Body)
			}
		}

		fmt.Println("Connection problem, retrying in 1s")
		time.Sleep(time.Second)
	}

	return nil, fmt.Errorf("no connection available")
}

// checkUser verifies if a Letterboxd username exists
func checkUser(username string) (string, bool) {
	// Check if username contains only alphanumeric chars (after removing underscores)
	usernameC := strings.ReplaceAll(username, "_", "")
	for _, c := range usernameC {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			fmt.Printf("The user \"%s\" does not exist.\n", username)
			return username, false
		}
	}

	// Check if the user exists on Letterboxd
	url := "https://letterboxd.com/" + username
	doc, err := getPage(url)
	if err != nil || doc == nil {
		fmt.Printf("The user \"%s\" does not exist.\n", username)
		return username, false
	}

	// Check if the page has the expected structure
	header := doc.Find("body header section")
	if header.Length() == 0 {
		fmt.Printf("The user \"%s\" does not exist.\n", username)
		return username, false
	}

	return username, true
}

// getUser prompts for and validates a username
func getUser() string {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\nYour Letterboxd Username:\n")
		user, _ := reader.ReadString('\n')
		user = strings.TrimSpace(user)

		if user == "" {
			continue
		}

		if validUser, ok := checkUser(user); ok {
			return validUser
		}

		fmt.Println("\nThis user does not exist!")
	}
}

// findFollowing gets all users the given user is following
func findFollowing(user string) []string {
	following := []string{}
	url := "https://letterboxd.com/" + user + "/following/"

	for {
		doc, err := getPage(url)
		if err != nil || doc == nil {
			break
		}

		doc.Find("td.table-person").Each(func(_ int, s *goquery.Selection) {
			if href, exists := s.Find("h3 a").Attr("href"); exists {
				userURL := strings.ReplaceAll(href, "/", "")
				following = append(following, userURL)
			}
		})

		nextLink, exists := doc.Find("div.pagination a.next").Attr("href")
		if !exists {
			break
		}
		url = "https://letterboxd.com" + nextLink
	}

	return following
}

// getFriends prompts for friends or gets them from following list
func getFriends(user string) []string {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("\nIf you don't want all your friends to be included, add just some users in the form of:")
		fmt.Println("\t\"user1, user2, user3\"")
		fmt.Print("Else just press Enter.\n")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		var friends []string
		if input == "" {
			fmt.Println("The friends list is generated...")
			friends = findFollowing(user)
		} else {
			fmt.Println("\nThe given users are checked...")
			for _, friend := range strings.Split(strings.ReplaceAll(input, " ", ""), ",") {
				if validFriend, ok := checkUser(friend); ok {
					friends = append(friends, validFriend)
				}
			}
		}

		if len(friends) == 0 {
			fmt.Println("\nNo user was found!")
			continue
		}

		return friends
	}
}

// getMovieCount gets the number of rated movies for each friend
func getMovieCount(friends []string) []int {
	fmt.Println("\nThe number of rated movies is collected...")
	movieCount := make([]int, len(friends))

	for i, friend := range friends {
		url := "https://letterboxd.com/" + friend + "/films/rated/.5-5/"
		doc, err := getPage(url)
		if err != nil || doc == nil {
			continue
		}

		// Try to find the count text
		text := doc.Find("span.replace-if-you").Parent().Text()
		parts := strings.Split(text, "has")
		if len(parts) < 2 {
			continue
		}

		// Extract numbers from the text
		var numb string
		for _, char := range parts[1] {
			if char >= '0' && char <= '9' {
				numb += string(char)
			}
		}

		if numb != "" {
			count, _ := strconv.Atoi(numb)
			movieCount[i] = count
		}
	}

	return movieCount
}

// askExcludeWatched asks if user's watched movies should be excluded
func askExcludeWatched() bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Should your watched movies be excluded from the list (y/n)?\n")
		exc, _ := reader.ReadString('\n')
		exc = strings.TrimSpace(exc)

		if exc == "n" {
			return false
		} else if exc == "y" {
			return true
		} else {
			fmt.Println("Please only enter \"y\" or \"n\"")
		}
	}
}

// getAllMovies gets all movies watched by a user
func getAllMovies(username string) []string {
	var movies []string
	fmt.Printf("All of '%s's' movies are searched...\n\n", username)

	url := "https://letterboxd.com/" + username + "/films/"
	for {
		doc, err := getPage(url)
		if err != nil || doc == nil {
			break
		}

		doc.Find("li.poster-container").Each(func(_ int, s *goquery.Selection) {
			if link, exists := s.Find("div").Attr("data-target-link"); exists {
				movies = append(movies, link)
			}
		})

		nextLink, exists := doc.Find("div.pagination a.next").Attr("href")
		if !exists {
			fmt.Printf("\"%s\" is finished.\n", username)
			fmt.Printf("%d movies were found\n\n", len(movies))
			return movies
		}
		url = "https://letterboxd.com" + nextLink
	}

	fmt.Printf("\"%s\" is finished.\n", username)
	fmt.Printf("%d movies were found\n\n", len(movies))
	return movies
}

// getRatedMovies gets all rated movies by a user, excluding specified movies
func getRatedMovies(username string, excludeMovies []string) []Movie {
	var movies []Movie
	fmt.Printf("All of \"%s\"s rated movies are searched...\n\n", username)

	excludeMap := make(map[string]bool)
	for _, m := range excludeMovies {
		excludeMap[m] = true
	}

	url := "https://letterboxd.com/" + username + "/films/by/member-rating/"
	for {
		doc, err := getPage(url)
		if err != nil || doc == nil {
			break
		}

		moviesOnPage := false
		doc.Find("li.poster-container").Each(func(_ int, s *goquery.Selection) {
			newTitle, exists := s.Find("div").Attr("data-target-link")
			if !exists {
				return
			}

			ratingElem := s.Find("p span.rating")
			if ratingElem.Length() == 0 {
				return
			}

			moviesOnPage = true
			ratingClass, exists := ratingElem.Attr("class")
			if !exists {
				return
			}

			parts := strings.Split(ratingClass, " ")
			ratingStr := strings.ReplaceAll(parts[len(parts)-1], "rated-", "")
			rating, err := strconv.Atoi(ratingStr)
			if err != nil {
				return
			}

			if !excludeMap[newTitle] {
				movies = append(movies, Movie{URL: newTitle, Rating: rating})
			}
		})

		if !moviesOnPage {
			fmt.Printf("\"%s\" is finished.\n", username)
			fmt.Printf("%d movies were found\n\n", len(movies))
			return movies
		}

		nextLink, exists := doc.Find("div.pagination a.next").Attr("href")
		if !exists {
			fmt.Printf("\"%s\" is finished.\n", username)
			fmt.Printf("%d movies were found\n\n", len(movies))
			return movies
		}
		url = "https://letterboxd.com" + nextLink
	}

	fmt.Printf("\"%s\" is finished.\n", username)
	fmt.Printf("%d movies were found\n\n", len(movies))
	return movies
}

// mergeMovies combines all movie ratings from different users
func mergeMovies(movies []Movie) []MovieWithRatings {
	// Sort by URL for easier grouping
	sort.Slice(movies, func(i, j int) bool {
		return movies[i].URL < movies[j].URL
	})

	var uniqueMovies []MovieWithRatings
	i := 0
	for i < len(movies) {
		movie := movies[i]
		ratings := []int{movie.Rating}

		j := i + 1
		for j < len(movies) && movies[j].URL == movie.URL {
			ratings = append(ratings, movies[j].Rating)
			j++
		}

		uniqueMovies = append(uniqueMovies, MovieWithRatings{
			URL:     movie.URL,
			Ratings: ratings,
		})

		i = j
	}

	return uniqueMovies
}

// processResults processes the merged movies data
func processResults(uniqueMovies []MovieWithRatings) []Result {
	var results []Result

	for _, movie := range uniqueMovies {
		avgRating := avg(movie.Ratings)
		results = append(results, Result{
			AvgRating: avgRating,
			VoteCount: len(movie.Ratings),
				 URL:       movie.URL,
				 Ratings:   movie.Ratings,
		})
	}

	return results
}

// checkNumber validates the threshold input
func checkNumber(thresholdStr string, friendsNr int) (int, bool) {
	threshold, err := strconv.Atoi(thresholdStr)
	if err != nil {
		fmt.Println("Please enter a whole number.")
		return 0, false
	}

	if threshold > friendsNr {
		fmt.Printf("Please enter a number smaller than %d.\n", friendsNr+1)
		return 0, false
	}

	return threshold, true
}

// showResults displays and handles results
func showResults(moviesList []Result, friendsNr int) {
	reader := bufio.NewReader(os.Stdin)
	threshold := 0

	for {
		fmt.Println("Minimum number of ratings per movie? (You can changes this later)")

		var thresholdStr string
		for threshold == 0 {
			fmt.Printf("Enter a number between 1 and %d.\n", friendsNr)
			thresholdStr, _ = reader.ReadString('\n')
			thresholdStr = strings.TrimSpace(thresholdStr)

			var valid bool
			threshold, valid = checkNumber(thresholdStr, friendsNr)
			if !valid {
				threshold = 0
			}
		}

		// Filter movies by threshold
		var moviesFiltered []Result
		for _, movie := range moviesList {
			if movie.VoteCount >= threshold {
				moviesFiltered = append(moviesFiltered, movie)
			}
		}

		// Sort movies by average rating and vote count
		sort.Slice(moviesFiltered, func(i, j int) bool {
			if moviesFiltered[i].AvgRating != moviesFiltered[j].AvgRating {
				return moviesFiltered[i].AvgRating > moviesFiltered[j].AvgRating
			}
			return moviesFiltered[i].VoteCount > moviesFiltered[j].VoteCount
		})

		moviesNr := len(moviesFiltered)
		fmt.Printf("\n\n%d movies have at least %d Vote(s)\n", moviesNr, threshold)
		fmt.Printf("Here are the top %d movie(s), sorted by average rating and number of votes.\n\n",
			   min(moviesNr, 15))

		fmt.Println("Avg\t Nr V, Titel,\t\t Individual Votes")
		for i := 0; i < min(moviesNr, 15); i++ {
			movie := moviesFiltered[i]
			movieName := strings.ReplaceAll(strings.ReplaceAll(movie.URL, "/film/", ""), "/", "")
			fmt.Printf("%.2f\t%d\t%s, %v\n", movie.AvgRating, movie.VoteCount, movieName, movie.Ratings)
		}
		fmt.Println("\n\n")

		fmt.Println("If you want to change the rating number, enter a new number.")
		fmt.Print("If you want to save the complete results write \"s\", if you want to end without saving press \"x\".\n")
		question, _ := reader.ReadString('\n')
		question = strings.TrimSpace(question)

		if question == "x" {
			fmt.Print("Are you sure you want to end without saving (y/n)?")
			r, _ := reader.ReadString('\n')
			r = strings.TrimSpace(r)
			if r == "y" {
				fmt.Println("\n --------------------------------END--------------------------------\n")
				return
			}
		} else if question == "s" {
			saveResults(moviesFiltered, threshold)
			return
		} else {
			threshold = 0
			thresholdStr = question
		}
	}
}

// saveResults saves the results to a CSV file
func saveResults(data []Result, threshold int) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("If you want to specifiy the dir and filename, enter it here.")
	fmt.Print("Else it will be saved as \"results.csv\" in the current dir\n")
	filename, _ := reader.ReadString('\n')
	filename = strings.TrimSpace(filename)

	if filename == "" {
		filename = "results.csv"
	}

	file, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{fmt.Sprintf("Movies with at least %d Votes, ranked by Avg and No. Votes.", threshold)})
	writer.Write([]string{"Avg Rating, No Votes, Movie, List of Votes"})

	for _, row := range data {
		// Convert ratings to strings
		ratings := make([]string, len(row.Ratings))
		for i, r := range row.Ratings {
			ratings[i] = strconv.Itoa(r)
		}

		writer.Write([]string{
			fmt.Sprintf("%.3f", row.AvgRating),
			     strconv.Itoa(row.VoteCount),
			     row.URL,
			     strings.Join(ratings, ", "),
		})
	}

	fmt.Println("List is saved")
}

// collectMoviesParallel collects movies from multiple users in parallel
func collectMoviesParallel(friends []string, excludeMovies []string) []Movie {
	var wg sync.WaitGroup
	moviesChan := make(chan []Movie, len(friends))

	// Calculate number of workers
	numWorkers := (len(friends) / 3) + 1
	if numWorkers > 12 {
		numWorkers = 12
	}
	if numWorkers < 2 {
		numWorkers = len(friends)
	}

	// Create a semaphore to limit concurrent requests
	semaphore := make(chan struct{}, numWorkers)

	for _, friend := range friends {
		wg.Add(1)
		go func(username string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			movies := getRatedMovies(username, excludeMovies)
			moviesChan <- movies
		}(friend)
	}

	// Wait for all goroutines to complete then close the channel
	go func() {
		wg.Wait()
		close(moviesChan)
	}()

	// Collect all movies
	var allMovies []Movie
	for movies := range moviesChan {
		allMovies = append(allMovies, movies...)
	}

	return allMovies
}

func main() {
	// Get user and friends
	user := getUser()
	friends := getFriends(user)
	movieCount := getMovieCount(friends)

	movieSum := 0
	for _, count := range movieCount {
		movieSum += count
	}
	fmt.Printf("%d movies were found.\n", movieSum)

	// Sort friends by movie count
	type FriendCount struct {
		Friend string
		Count  int
	}

	combinedList := make([]FriendCount, len(friends))
	for i := range friends {
		combinedList[i] = FriendCount{Friend: friends[i], Count: movieCount[i]}
	}

	sort.Slice(combinedList, func(i, j int) bool {
		return combinedList[i].Count > combinedList[j].Count
	})

	// Update friends list to sorted order
	friends = make([]string, len(combinedList))
	for i, fc := range combinedList {
		friends[i] = fc.Friend
	}

	fmt.Println("\n\nThese eligible users were given:")
	for _, fc := range combinedList {
		fmt.Printf("%s, %d rated movies\n", fc.Friend, fc.Count)
	}
	fmt.Println("\n\n")

	// Check if user wants to exclude their watched movies
	var myMovies []string
	excludeWatched := askExcludeWatched()
	if excludeWatched {
		myMovies = getAllMovies(user)
		fmt.Printf("%d movies found. These will be excluded.\n\n", len(myMovies))
	}

	// Warning for large number of movies
	if movieSum > 3000 {
		fmt.Printf("\n%d movies will be searched.\n", movieSum)
		maxCount := 0
		for _, count := range movieCount {
			if count > maxCount {
				maxCount = count
			}
		}
		fmt.Printf("This could take a while, estimated time: %.1f min.\n", math.Max(float64(maxCount)/3000, 0.1))

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Do you want to start? (y/n)\n")
		start, _ := reader.ReadString('\n')
		start = strings.TrimSpace(start)

		if !strings.Contains(start, "y") {
			os.Exit(0)
		}
	}

	// Collect movies in parallel
	allMovies := collectMoviesParallel(friends, myMovies)

	// Merge and process movies
	fmt.Println("All ratings are combined...")
	uniqueMovies := mergeMovies(allMovies)
	fmt.Printf("%d unique and rated movies are found.\n\n", len(uniqueMovies))

	results := processResults(uniqueMovies)
	showResults(results, len(friends))
}
