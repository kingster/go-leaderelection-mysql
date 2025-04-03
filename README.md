# Go Leader Election MySQL

This library provides a simple leader election mechanism using MySQL as a backend. It allows multiple instances of an application to coordinate and elect a single leader among them.

## Features

*   Uses MySQL for distributed locking and leader election.
*   Automatic renewal of leadership lease.
*   Callback functions for becoming a leader and losing leadership.
*   Unique worker identification based on hostname, MAC address, and PID.

## Configuration

The library requires the following environment variables to be set for MySQL database connection. You can place these in a `.env` file in your project root.

```dotenv
MYSQL_USER=your_mysql_user
MYSQL_PASSWORD=your_mysql_password
MYSQL_HOST=your_mysql_host
MYSQL_PORT=3306
MYSQL_DBNAME=your_mysql_database
```

## Usage

Import the library and use the `ElectLeader` function to participate in an election.

```go
package main

import (
	"fmt"
	"time"

	leaderelection "github.com/your_github_username/go-leaderelection-mysql" // Replace with your actual module path
)

func main() {
	electionName := "my-critical-task"

	becomeLeader := func() {
		fmt.Println("I am the leader now! Performing critical tasks...")
		// Add your leader-specific logic here
	}

	loseLeadership := func() {
		fmt.Println("I lost leadership. Standing by...")
		// Add logic for when leadership is lost
	}

	// Start the election process. This function blocks indefinitely.
	leaderelection.ElectLeader(electionName, becomeLeader, loseLeadership)

	// Keep the main goroutine alive (optional, as ElectLeader blocks)
	select {}
}

```

### How it Works

1.  **Initialization**: `NewElection` connects to the MySQL database specified by the environment variables and performs auto-migration to ensure the `election_records` table exists.
2.  **Worker Identification**: Each candidate instance identifies itself with a unique `workerName` generated from the hostname, MAC addresses, and process ID.
3.  **Campaigning**: The `Campaign` method attempts to acquire or renew the leadership lease in the `election_records` table. It uses an `INSERT IGNORE ... ON DUPLICATE KEY UPDATE` SQL statement.
    *   If the `INSERT IGNORE` succeeds, the candidate becomes the leader immediately.
    *   If the row already exists (`ON DUPLICATE KEY UPDATE`), it checks if the `last_update` timestamp is older than 60 seconds. If it is, it means the previous leader's lease has expired, and the current candidate takes over leadership by updating the `leader_name` and `last_update`.
    *   If the existing `leader_name` matches the candidate's name, it simply updates the `last_update` timestamp to renew the lease.
4.  **Lease Renewal**: The leading instance periodically calls `Campaign` (every 15 seconds in `ElectLeader`) to renew its lease by updating the `last_update` timestamp.
5.  **Leadership Loss**: If a candidate fails to acquire or renew the lease (e.g., another instance became the leader or renewed its lease), it enters a waiting state (60 seconds in `ElectLeader`) before retrying. If it was previously the leader, the `loseLeadership` callback is invoked.
6.  **Callbacks**: The `becomeLeaderCb` is called when an instance successfully acquires leadership. The `looseLeadershipCB` is called when a leading instance fails to renew its lease.

## Dependencies

*   [gorm.io/driver/mysql](https://gorm.io/docs/connecting_to_the_database.html#MySQL)
*   [gorm.io/gorm](https://gorm.io/)
*   [github.com/joho/godotenv](https://github.com/joho/godotenv) (for loading .env files in the example, not strictly required by the library itself if config is passed differently)

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests. 