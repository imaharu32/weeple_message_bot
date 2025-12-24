import { useState } from 'react'
import { collection, addDoc } from "firebase/firestore"
import { db } from "./firebase"
import type { ChannelType } from "./App"
import './ReminderForm.css'

interface ReminderFormProps {
  channelOptions: Array<{ label: string; channelType: ChannelType }>
}

export function ReminderForm({ channelOptions }: ReminderFormProps) {
  const [reminderMessage, setReminderMessage] = useState<string>('')
  const [reminderDate, setReminderDate] = useState<string>('')
  const [reminderTime, setReminderTime] = useState<string>('')
  const [selectedChannel, setSelectedChannel] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [response, setResponse] = useState<string>('')
  const [error, setError] = useState<string>('')

  const handleAddReminder = async () => {
    if (!reminderMessage.trim()) {
      setError('リマインド内容を入力してください')
      return
    }
    if (!reminderDate || !reminderTime) {
      setError('リマインド日時を設定してください')
      return
    }
    if (!selectedChannel) {
      setError('送信先チャンネルを選択してください')
      return
    }

    setLoading(true)
    setError('')
    setResponse('')

    try {
      const reminderDateTime = new Date(`${reminderDate}T${reminderTime}`)
      
      if (reminderDateTime <= new Date()) {
        setError('未来の日時を設定してください')
        setLoading(false)
        return
      }

      await addDoc(collection(db, "reminders"), {
        message: reminderMessage,
        scheduledAt: reminderDateTime,
        channelType: selectedChannel as ChannelType,
        status: 'pending',
        createdAt: new Date()
      })

      setResponse('リマインダーを登録しました！')
      setReminderMessage('')
      setReminderDate('')
      setReminderTime('')
      setSelectedChannel('')
    } catch (err) {
      console.error('Error adding reminder:', err)
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="reminder-form">
      <h2>リマインダー登録</h2>
      
      <div className="form-group">
        <label htmlFor="reminder-message">リマインド内容:</label>
        <textarea
          id="reminder-message"
          value={reminderMessage}
          onChange={(e) => setReminderMessage(e.target.value)}
          disabled={loading}
          placeholder="リマインドする内容を入力してください..."
          className="textarea-input"
          rows={4}
        />
      </div>

      <div className="form-row">
        <div className="form-group">
          <label htmlFor="reminder-date">リマインド日付:</label>
          <input
            type="date"
            id="reminder-date"
            value={reminderDate}
            onChange={(e) => setReminderDate(e.target.value)}
            disabled={loading}
            className="date-input"
          />
        </div>

        <div className="form-group">
          <label htmlFor="reminder-time">リマインド時刻:</label>
          <input
            type="time"
            id="reminder-time"
            value={reminderTime}
            onChange={(e) => setReminderTime(e.target.value)}
            disabled={loading}
            className="time-input"
          />
        </div>
      </div>

      <div className="form-group">
        <label htmlFor="reminder-channel">送信先チャンネル:</label>
        <select
          id="reminder-channel"
          value={selectedChannel}
          onChange={(e) => setSelectedChannel(e.target.value)}
          disabled={loading}
          className="select-input"
        >
          <option value="">-- チャンネルを選択してください --</option>
          {channelOptions.map((option, index) => (
            <option key={index} value={option.channelType}>
              {option.label}
            </option>
          ))}
        </select>
      </div>

      <button
        onClick={handleAddReminder}
        disabled={loading || !reminderMessage.trim() || !reminderDate || !reminderTime || !selectedChannel}
        className="reminder-button"
      >
        {loading ? 'リマインダー登録中...' : 'リマインダーを登録'}
      </button>

      {error && <div className="error">{error}</div>}
      {response && <div className="success">{response}</div>}
    </div>
  )
}
