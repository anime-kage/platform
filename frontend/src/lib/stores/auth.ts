import { writable } from 'svelte/store';
import type { User } from '$shared/types';
import { api } from '../api';

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
}

const initialState: AuthState = {
  user: null,
  isAuthenticated: false,
  isLoading: true,
};

function createAuthStore() {
  const { subscribe, set, update } = writable<AuthState>(initialState);

  return {
    subscribe,
    
    async init() {
      update(state => ({ ...state, isLoading: true }));
      
      try {
        if (api.isAuthenticated()) {
          const user = await api.getCurrentUser();
          // Validate user object before setting
          if (user && user.username && user.email) {
            set({
              user,
              isAuthenticated: true,
              isLoading: false,
            });
          } else {
            // Invalid user data, clear auth
            api.clearToken();
            set({
              user: null,
              isAuthenticated: false,
              isLoading: false,
            });
          }
        } else {
          set({
            user: null,
            isAuthenticated: false,
            isLoading: false,
          });
        }
      } catch (error) {
        console.error('Auth init error:', error);
        api.clearToken();
        set({
          user: null,
          isAuthenticated: false,
          isLoading: false,
        });
      }
    },

    async login(email: string, password: string) {
      try {
        const response = await api.login({ email, password });
        // Validate response before setting
        if (response && response.user && response.user.username && response.user.email) {
          set({
            user: response.user,
            isAuthenticated: true,
            isLoading: false,
          });
          return response;
        } else {
          throw new Error('Invalid login response');
        }
      } catch (error) {
        set({
          user: null,
          isAuthenticated: false,
          isLoading: false,
        });
        throw error;
      }
    },

    async register(username: string, email: string, password: string, confirmPassword: string, inviteCode?: string) {
      try {
        const response = await api.register({ username, email, password, confirmPassword, inviteCode });
        // Validate response before setting
        if (response && response.user && response.user.username && response.user.email) {
          set({
            user: response.user,
            isAuthenticated: true,
            isLoading: false,
          });
          return response;
        } else {
          throw new Error('Invalid registration response');
        }
      } catch (error) {
        set({
          user: null,
          isAuthenticated: false,
          isLoading: false,
        });
        throw error;
      }
    },

    async logout() {
      try {
        await api.logout();
      } catch (error) {
        console.error('Logout error:', error);
      } finally {
        set({
          user: null,
          isAuthenticated: false,
          isLoading: false,
        });
      }
    },

    updateUser(user: User) {
      // Validate user object before updating
      if (user && user.username && user.email) {
        update(state => ({
          ...state,
          user,
        }));
      } else {
        console.warn('Invalid user object provided to updateUser');
      }
    },
  };
}

export const authStore = createAuthStore();