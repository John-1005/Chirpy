This was a project to help me learn http.Servers from boot.dev
I learned how to make and call API, how to handle requests. I also learned how to do a basic auth setup with hashing. 
Helped me learn about webhooks and how to implement into a project
Also helped me learn how to better marshal and unmarshal JSON while accessing a sql database

I feel like this was very useful and I learned a ton on how to build and how it works. I am saving this so I will always have a reference.


Everything in this project is an internal setup accessing a local host


You will need sqlc and gator in order to use the "chirp json storage"



Step 1: Install Golang

Please follow the instructions located here: https://golang.org/dl/.

Step 2: Install Gator

To install the Gator CLI tool, use the following go install command:

go install github.com/AleksZieba/gator@latest

Step 3: Set up the Configuration File

Create a .gatorc

{ "db_url": "postgresql://USERNAME:PASSWORD@localhost:5432/DBNAME?sslmode=disable", "current_user_name": "YOUR_USER_NAME" }

Please be sure to replace USERNAME, PASSWORD, DBNAME and YOUR_USER_NAME with the appropriate values.

--------------------------------


After installing Gator you will need to manually create a config file in your home directory, ~/.gatorconfig.json that has the following:

{
  "db_url": "connection_string_goes_here",
}

--------------------------------

