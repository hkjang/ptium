import React from 'react'
import ReactDOM from 'react-dom/client'
import { App } from './App'
import { AuthProvider } from './auth/AuthContext'
import { BrandProvider } from './branding/BrandContext'
import { ToastProvider } from './components/Toast'
import './styles.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ToastProvider>
      <BrandProvider>
        <AuthProvider>
          <App />
        </AuthProvider>
      </BrandProvider>
    </ToastProvider>
  </React.StrictMode>,
)
