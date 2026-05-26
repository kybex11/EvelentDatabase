import { DBClient } from "./client";

const db = new DBClient("http://localhost:8080");

const collections = db.listCollections();
console.log(collections);
