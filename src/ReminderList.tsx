import { useState, useEffect } from 'react'
import { collection, query, orderBy, onSnapshot, deleteDoc, doc} from "firebase/firestore"
import { db } from "./firebase"
import type { ChannelType } from "./App"
import { ReminderForm } from "./ReminderForm"
import './ReminderList.css'

interface Reminder {
  id: string
  message: string
  scheduledAt: Date
  channelType: ChannelType
  status: 'pending' | 'sent' | 'failed'
  createdAt: Date
}

interface ReminderListProps {
  channelOptions: Array<{ label: string; channelType: ChannelType }>
}

export function ReminderList({ channelOptions }: ReminderListProps) {
  const [reminders, setReminders] = useState<Reminder[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>('')
  const [showForm, setShowForm] = useState(false)

  useEffect(() => {
    const q = query(collection(db, "reminders"), orderBy("scheduledAt", "asc"))
    
    const unsubscribe = onSnapshot(q, (querySnapshot) => {
      const reminderList: Reminder[] = []
      querySnapshot.forEach((doc) => {
        const data = doc.data()
        reminderList.push({
          id: doc.id,
          message: data.message,
          scheduledAt: data.scheduledAt.toDate(),
          channelType: data.channelType,
          status: data.status,
          createdAt: data.createdAt.toDate()
        })
      })
      setReminders(reminderList)
      setLoading(false)
    }, (err) => {
      console.error("Error fetching reminders:", err)
      setError("リマインダーの取得に失敗しました")
      setLoading(false)
    })

    return () => unsubscribe()
  }, [])

  const handleDelete = async (id: string) => {
    if (!confirm('このリマインダーを削除しますか？')) {
      return
    }

    try {
      await deleteDoc(doc(db, "reminders", id))
    } catch (err) {
      console.error("Error deleting reminder:", err)
      setError("削除に失敗しました")
    }
  }


  const formatDateTime = (date: Date) => {
    const year = date.getFullYear()
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    const hours = String(date.getHours()).padStart(2, '0')
    const minutes = String(date.getMinutes()).padStart(2, '0')
    return `${year}/${month}/${day} ${hours}:${minutes}`
  }

  const getChannelLabel = (channelType: ChannelType) => {
    const option = channelOptions.find(opt => opt.channelType === channelType)
    return option?.label || channelType
  }

  const getStatusLabel = (status: string) => {
    switch(status) {
      case 'pending': return '待機中'
      case 'sent': return '送信済み'
      case 'failed': return '失敗'
      default: return status
    }
  }

  const getStatusClass = (status: string) => {
    switch(status) {
      case 'pending': return 'status-pending'
      case 'sent': return 'status-sent'
      case 'failed': return 'status-failed'
      default: return ''
    }
  }

  if (loading) {
    return <div className="reminder-list-container"><p>読み込み中...</p></div>
  }

  return (
    <div className="reminder-list-container">
      <div className="reminder-list-header">
        <h2>登録済みリマインダー一覧</h2>
        <button 
          className="add-reminder-button"
          onClick={() => setShowForm(!showForm)}
        >
          {showForm ? 'フォームを閉じる' : '+ リマインダーを登録'}
        </button>
      </div>

      {showForm && (
        <ReminderForm 
          channelOptions={channelOptions}
        />
      )}

      {error && <div className="error">{error}</div>}

      {reminders.length === 0 ? (
        <p className="no-reminders">登録されているリマインダーはありません</p>
      ) : (
        <div className="reminders-grid">
          {reminders.map((reminder) => (
            <div key={reminder.id} className="reminder-card">
              <div className="reminder-header">
                <span className={`reminder-status ${getStatusClass(reminder.status)}`}>
                  {getStatusLabel(reminder.status)}
                </span>
                <span className="reminder-channel">{getChannelLabel(reminder.channelType)}</span>
              </div>
              
              <div className="reminder-datetime">
                <strong>📅 {formatDateTime(reminder.scheduledAt)}</strong>
              </div>
              
              <div className="reminder-message">
                {reminder.message}
              </div>
              
              <div className="reminder-actions">
                <button 
                  className="delete-button"
                  onClick={() => handleDelete(reminder.id)}
                >
                  削除
                </button>
              </div>
              
              <div className="reminder-created">
                登録日時: {formatDateTime(reminder.createdAt)}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
