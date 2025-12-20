import { collection, getDocs, query, orderBy } from "firebase/firestore";
import type { Timestamp } from "firebase/firestore";
import { db } from "./firebase";
import type { ChannelType } from "./App";

export interface HistoryRecord {
    fire_id: string;
    id: string;
    message: string;
    createdAt: Timestamp;
}

export async function getHistory(type: ChannelType): Promise<HistoryRecord[]> {
    const collectionName = type + "_messages";
    const q = query(collection(db, collectionName), orderBy("createdAt", "desc"));
    const querySnapshot = await getDocs(q);
    return querySnapshot.docs.map(doc => {
        const data = doc.data();
        return {
            fire_id: doc.id,
            id: data.id || doc.id,
            message: data.message,
            createdAt: data.createdAt
        } as HistoryRecord;
    });
}