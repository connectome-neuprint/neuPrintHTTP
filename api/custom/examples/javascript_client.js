/**
 * JavaScript client example for neuPrintHTTP Arrow format.
 * 
 * This example demonstrates:
 * - Using HTTP Arrow IPC format for direct query results in JavaScript
 * 
 * Requirements:
 * - npm install apache-arrow
 * 
 * Note: This example uses ES modules. Run with:
 * - node --experimental-modules javascript_client.js
 * - Or use in a browser environment
 */

import { tableFromIPC } from 'apache-arrow';

/**
 * Execute a query and receive the results as an Arrow IPC stream over HTTP.
 * 
 * @param {string} server - Base URL of the neuPrintHTTP server
 * @param {string} dataset - Dataset name
 * @param {string} cypherQuery - The Cypher query to execute
 * @param {string|null} jwtToken - Optional JWT token for authenticated access
 * @returns {Promise<import('apache-arrow').Table>} - Arrow Table containing the results
 */
async function queryArrowIpcStream(server, dataset, cypherQuery, jwtToken = null) {
  // Prepare the request
  const url = `${server}/api/custom/arrow`;
  const headers = {
    'Content-Type': 'application/json',
  };

  // Add authorization if token provided
  if (jwtToken) {
    headers['Authorization'] = `Bearer ${jwtToken}`;
  }

  const data = {
    cypher: cypherQuery,
    dataset: dataset
  };

  console.log(`Sending request to ${url}`);

  // Make the request
  const response = await fetch(url, {
    method: 'POST',
    headers: headers,
    body: JSON.stringify(data)
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Query failed with status ${response.status}: ${errorText}`);
  }

  const contentType = response.headers.get('Content-Type') || '';
  if (!contentType.includes('application/vnd.apache.arrow.stream')) {
    throw new Error(`Expected Arrow stream content type but got: ${contentType}`);
  }

  // Process the Arrow IPC stream
  const arrayBuffer = await response.arrayBuffer();
  const table = await tableFromIPC(arrayBuffer);

  console.log(`Received Arrow table with ${table.numRows} rows and ${table.numCols} columns`);
  console.log(`Schema: ${table.schema.toString()}`);

  return table;
}

/**
 * Example function that demonstrates the HTTP Arrow IPC stream usage
 */
async function httpIpcExample() {
  // Configuration
  const server = "http://localhost:11000";
  const dataset = "hemibrain";

  // Example Cypher query - modify this to match your dataset
  const cypherQuery = `
    MATCH (n) 
    RETURN n.type AS type, count(*) AS count 
    ORDER BY count DESC 
    LIMIT 10
  `;

  try {
    console.log("\n===== HTTP Arrow IPC Stream Example (JavaScript) =====");
    console.log(`Query: ${cypherQuery}`);

    const table = await queryArrowIpcStream(server, dataset, cypherQuery);

    // Convert to a more JS-friendly format for display
    const results = [];
    const columnNames = table.schema.fields.map(f => f.name);
    
    for (let i = 0; i < table.numRows; i++) {
      const row = {};
      for (const name of columnNames) {
        row[name] = table.getChild(name).get(i);
      }
      results.push(row);
    }

    console.log("\nResults as JSON:");
    console.log(JSON.stringify(results, null, 2));

    // Example data analysis
    if (results.length > 0 && 'count' in results[0]) {
      console.log("\nExample data analysis:");
      const totalCount = results.reduce((sum, row) => sum + row.count, 0);
      console.log(`Total count: ${totalCount}`);

      console.log("\nDistribution:");
      const maxCount = Math.max(...results.map(row => row.count));
      const maxWidth = 40;

      for (const row of results) {
        const label = String(row.type || 'unknown').substring(0, 20).padEnd(20);
        const count = row.count;
        const barWidth = Math.floor((count / maxCount) * maxWidth);
        const bar = '█'.repeat(barWidth);
        console.log(`${label} | ${String(count).padStart(6)} | ${bar}`);
      }
    }

    return true;
  } catch (error) {
    console.error(`HTTP IPC stream error: ${error.message}`);
    return false;
  }
}

// Run the example if this file is executed directly
if (typeof require !== 'undefined' && require.main === module) {
  httpIpcExample().catch(console.error);
}

// Export for use in modules
export {
  queryArrowIpcStream,
  httpIpcExample
};