package mongo

import (
	"context"
	"errors"

	"github.com/pejeio/blood-donate-locator-api/internal/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Client struct {
	Database *mongo.Database
}

func (c *Client) LocationsCollection() *mongo.Collection {
	return c.Database.Collection("locations")
}

func (c *Client) CreateLocationIndexes(ctx context.Context) error {
	geoJSONIdxModel := mongo.IndexModel{
		Keys: bson.D{primitive.E{Key: "geometry", Value: "2dsphere"}},
	}
	_, err := c.LocationsCollection().Indexes().CreateOne(ctx, geoJSONIdxModel)
	return err
}

func (c *Client) ReverseGeoCodeLocations(ctx context.Context, query types.LookupLocationRequest) ([]types.Location, error) {
	mongoDBHQ := bson.D{
		primitive.E{Key: "type", Value: "Point"},
		primitive.E{Key: "coordinates", Value: []float64{query.Latitude, query.Longitude}},
	}
	filter := bson.D{
		primitive.E{
			Key: "geometry", Value: bson.D{
				primitive.E{
					Key: "$near",
					Value: bson.D{
						primitive.E{Key: "$geometry", Value: mongoDBHQ},
						primitive.E{Key: "$maxDistance", Value: query.MaxDistance * 1000},
					},
				},
			},
		},
	}
	cursor, err := c.LocationsCollection().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	locations := make([]types.Location, 0)

	for cursor.Next(ctx) {
		var location types.Location
		err := cursor.Decode(&location)
		if err != nil {
			return nil, err
		}
		locations = append(locations, location)
	}

	return locations, nil
}

func (c *Client) GetLocations(ctx context.Context, query types.FindLocationsRequest) ([]types.Location, error) {
	opts := options.Find()
	opts.SetSort(bson.D{{Key: "created_at", Value: -1}})
	opts.SetLimit(int64(query.Limit))
	opts.SetSkip(int64(query.Offset))

	filter := bson.M{}
	if query.City != "" {
		filter["address.city"] = query.City
	}
	if query.PostalCode != "" {
		filter["address.postal_code"] = query.PostalCode
	}

	locations := make([]types.Location, 0)

	cursor, err := c.LocationsCollection().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}

	for cursor.Next(ctx) {
		var location types.Location
		err := cursor.Decode(&location)
		if err != nil {
			return nil, err
		}
		locations = append(locations, location)
	}

	return locations, nil
}

func (c *Client) GetLocationByID(ctx context.Context, id string) (types.Location, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return types.Location{}, err
	}
	filter := bson.M{"_id": oid}
	res := c.LocationsCollection().FindOne(ctx, filter)
	if res.Err() != nil {
		if res.Err() == mongo.ErrNoDocuments {
			return types.Location{}, errors.New("not found")
		}
		return types.Location{}, res.Err()
	}
	location := types.Location{}
	err = res.Decode(&location)
	if err != nil {
		return types.Location{}, err
	}

	return location, nil
}

func (c *Client) CreateLocation(ctx context.Context, loc types.CreateLocationRequest) (*types.Location, error) {
	newLocation := types.Location{
		Name: loc.Name,
		Geometry: &types.GeoJSONPoint{
			Type:        "Point",
			Coordinates: [2]float64{loc.Coordinates.Longitude, loc.Coordinates.Latitude},
		},
		Address: loc.Address,
	}
	_, err := newLocation.MarshalBSON()
	if err != nil {
		return nil, err
	}
	doc, err := c.LocationsCollection().InsertOne(ctx, newLocation)
	if err != nil {
		return nil, err
	}
	newLocation.ID = doc.InsertedID.(primitive.ObjectID)

	return &newLocation, err
}

func (c *Client) DeleteLocation(ctx context.Context, id string) (int, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return 0, errors.New("invalid ID")
	}
	filter := bson.M{"_id": objID}
	res, err := c.LocationsCollection().DeleteOne(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int(res.DeletedCount), nil
}

func (c *Client) CountLocations(ctx context.Context) (int64, error) {
	filter := bson.D{}

	return c.LocationsCollection().CountDocuments(ctx, filter)
}
