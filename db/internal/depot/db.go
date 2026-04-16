package depot

import "go.mongodb.org/mongo-driver/v2/mongo"

var (
	// MongoDB client
	Client *mongo.Client
	// Server config database
	Solis *mongo.Database
	// Master data database
	Master *mongo.Database
	// User data database
	User *mongo.Database
)
