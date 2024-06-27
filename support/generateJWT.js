// Use this code to generate a JWT that can access the users list stored
// on google cloud. 


const fs = require('fs');
const jwt = require('jsonwebtoken');

// Function to read app secret securely from config file
function getAppSecret(configPath) {
  try {
    const configData = fs.readFileSync(configPath, 'utf8');
    console.log(configData);
    const config = JSON.parse(configData);
    return config.appsecret;
  } catch (error) {
    console.error('Error reading app secret:', error.message);
    process.exit(1); // Exit with error code if config file or secret is missing
  }
}

// Function to generate JWT
function generateJWT(email, expirationDate, appSecret) {
  const payload = {
    email,
    level: 'admin',
    'image-url': 'https://lh4.googleusercontent.com/-TAI1cI0EqL8/AAAAAAAAAAI/AAAAAAABmdI/NiyORSV-9Mg/photo.jpg?sz=50',
    exp: Math.floor(expirationDate.getTime() / 1000), // Convert date to Unix timestamp in seconds
  };

  return jwt.sign(payload, appSecret);
}

// Script execution
if (process.argv.length < 5) {
  console.error('Usage: node generate_jwt.js <email> <expiration_date> <config_path>');
  process.exit(1);
}

const email = process.argv[2];
const expirationDate = new Date(process.argv[3]);
const configPath = process.argv[4];

if (expirationDate < new Date()) {
  console.error('Expiration date must be in the future');
  process.exit(1);
}

const appSecret = getAppSecret(configPath);
const token = generateJWT(email, expirationDate, appSecret);

console.log('Generated JWT:', token);

