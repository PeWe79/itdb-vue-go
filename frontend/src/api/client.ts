import axios from 'axios';
import { useNoticeStore } from '../stores/notice';

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
});

api.interceptors.request.use(config => {
  const token = localStorage.getItem('itdb_token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

api.interceptors.response.use(
  response => response,
  error => {
    const noticeStore = useNoticeStore();
    const responseMessage = (error as { response?: { data?: { error?: string } } })?.response?.data
      ?.error;
    const status = (error as { response?: { status?: number } })?.response?.status;
    const code = (error as { code?: string })?.code;

    let message = (responseMessage ?? '').trim();
    if (!message) {
      if (code === 'ECONNABORTED') {
        message = 'Request timed out; please try again later.';
      } else if (typeof navigator !== 'undefined' && navigator && navigator.onLine === false) {
        message = 'Network is unavailable; please check your network connection.';
      } else if (status === 401) {
        message = 'Session has expired; please log in again.';
        localStorage.removeItem('itdb_token');
        setTimeout(() => {
          window.location.href = '/login';
        }, 800);
      } else if (status === 403) {
        message = 'You do not have permission to perform this action.';
      } else if (status === 404) {
        message = 'The requested endpoint does not exist.';
      } else if (typeof status === 'number' && status >= 500) {
        message = 'Internal server error; please try again later.';
      } else {
        message = 'Operation failed; please try again later.';
      }
    }

    noticeStore.error(message);
    return Promise.reject(error);
  }
);

export default api;
